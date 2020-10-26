package enrichers

import (
	"fmt"
	"github.com/alouca/gosnmp"
	flow "github.com/bwNetFlow/protobuf/go"
	cache "github.com/patrickmn/go-cache"
	"log"
	"net"
	"regexp"
	"strings"
	"time"
)

var (
	snmpCache     *cache.Cache
	ifdescRegex   *regexp.Regexp
	snmpCommunity string

	oidBase = ".1.3.6.1.2.1.31.1.1.1.%d.%d"
	oidExts = map[string]uint8{"name": 1, "speed": 15, "desc": 18}

	snmpSemaphore = make(chan struct{}, 64)
)

type cacheEntry struct {
	router string
	oid    string
	answer interface{}
}

// Refresh OID cache periodically. Needs no syncing or waiting, it will die
// when main() exits. Tries to refresh all entries, ignores errors. If an entry
// errors multiple times in a row, it will be expunged eventually.
func refreshLoop() {
	log.Println("SNMP refresh: Started.")
	for {
		time.Sleep(1 * time.Hour)
		log.Printf("SNMP refresh: Starting hourly run with %d items.\n", snmpCache.ItemCount())
		for key, item := range snmpCache.Items() {
			content := item.Object.(cacheEntry)
			// This check is needed for blank entries. These happen
			// when a querySNMP was invoked, but the entry is still
			// blank because of the rate limit
			if content == *new(cacheEntry) {
				log.Println("SNMP refresh: Found blank key, initial Get pending.")
				continue
			}
			s, err := gosnmp.NewGoSNMP(content.router, snmpCommunity, gosnmp.Version2c, 1)
			if err != nil {
				log.Println("SNMP refresh: Connection Error:", err)
				continue
			}
			resp, err := s.Get(content.oid)
			if err != nil {
				log.Printf("SNMP refresh: Get failed for '%s' from %s.", content.oid, content.router)
				log.Printf("              Error Message: %s\n", err)
				continue
			}

			// parse and cache
			if len(resp.Variables) == 1 {
				new_answer := resp.Variables[0].Value
				// this is somewhat ugly, maybe place this elsewhere?
				if strings.Contains(key, "desc") {
					actual := ifdescRegex.FindStringSubmatch(new_answer.(string))
					if actual != nil && len(actual) > 1 {
						new_answer = actual[1]
					}
				}
				if new_answer != content.answer {
					// .Replace actually, but Set does not
					// error if the key expired meanwhile
					snmpCache.Set(
						key,
						cacheEntry{content.router, content.oid, new_answer},
						cache.DefaultExpiration)
					log.Printf("SNMP refresh: Updated '%s' from %s.\n", content.oid, content.router)
				} else {
					// reset expire
					snmpCache.Set(key, content, cache.DefaultExpiration)
				}
			} else {
				log.Printf("SNMP refresh: Bad response: %v", resp.Variables)
			}
		}
		log.Println("SNMP refresh: Finished hourly run.")
	}
}

// Query a single SNMP datapoint and cache the result. Supposedly a short-lived
// goroutine.
func querySNMP(router string, iface uint32, datapoint string) {
	oid := fmt.Sprintf(oidBase, oidExts[datapoint], iface)
	snmpSemaphore <- struct{}{} // acquire

	s, err := gosnmp.NewGoSNMP(router, snmpCommunity, gosnmp.Version2c, 1)
	if err != nil {
		log.Println("SNMP: Connection Error:", err)
		<-snmpSemaphore // release
		return
	}
	resp, err := s.Get(oid)
	if err != nil {
		log.Printf("SNMP: Get failed for '%s' from %s.", oid, router)
		log.Printf("      Error Message: %s\n", err)
		<-snmpSemaphore // release
		return
	}

	// release semaphore, UDP sockets are closed
	<-snmpSemaphore

	// parse and cache
	if len(resp.Variables) == 1 {
		value := resp.Variables[0].Value // TODO: check data types maybe
		// this is somewhat ugly, maybe place this elsewhere?
		if datapoint == "desc" {
			actual := ifdescRegex.FindStringSubmatch(value.(string))
			if actual != nil && len(actual) > 1 {
				value = actual[1]
			}
		}
		snmpCache.Set(fmt.Sprintf("%s %d %s", router, iface, datapoint), cacheEntry{router, oid, value}, cache.DefaultExpiration)
		// log.Printf("SNMP: Retrieved '%s' from %s.\n", oid, router)

	} else {
		log.Printf("SNMP: Bad response: %v", resp.Variables)
	}
}

// Init Caching, compile the Regex and set the SNMP Community global. Also set
// up hourly refreshs of the cached data.
func InitSnmp(regex string, community string) {
	snmpCache = cache.New(3*time.Hour, 1*time.Hour)
	ifdescRegex, _ = regexp.Compile(regex)
	snmpCommunity = community

	go refreshLoop()
}

// Get all three datapoints from cache, or initiate a SNMP query to populate
// it. Returns an uninitialized value per initiated SNMP request, but typically
// all three datapoints will be populated at the same time.
func getCachedOrQuery(router string, iface uint32) (string, string, uint32) {
	var name, desc string
	var speed uint32
	for datapoint, _ := range oidExts {
		if value, found := snmpCache.Get(fmt.Sprintf("%s %d %s", router, iface, datapoint)); found {
			entry := value.(cacheEntry)
			if entry.answer == nil {
				// we do not need to jump into else, as a
				// querySNMP goroutine is triggered already
				continue
			}
			switch datapoint {
			case "name":
				name = entry.answer.(string)
			case "desc":
				desc = entry.answer.(string)
			case "speed":
				speed = uint32(entry.answer.(uint64))
			}
		} else {
			snmpCache.Set(fmt.Sprintf("%s %d %s", router, iface, datapoint), cacheEntry{}, cache.DefaultExpiration)
			go querySNMP(router, iface, datapoint)
		}
	}
	return name, desc, speed
}

// Annotate a flow with data from SNMP. If there is no information in the
// cache, uninitialized values will be set and a SNMP request will be created
// in the background. Adds Iface Name, Desc and Speed.
func AddSnmp(flowmsg flow.FlowMessage) *flow.FlowMessage {
	// OIDs for Src and Dst, according to the following examples
	//  .1.3.6.1.2.1.31.1.1.1.1.<snmp_id> -> TenGigE0/0/0/3
	// .1.3.6.1.2.1.31.1.1.1.15.<snmp_id> -> 10000
	// .1.3.6.1.2.1.31.1.1.1.18.<snmp_id> -> _id_stuffs_ XX Telia Sonera (provider line codes)
	//					 |^^^^^^^^^^ |^ |^^^^^^^^^^^  |^^^^^^^^^^^^^^^^^^
	//					 useless  speed actual desc   semi private
	// The interface description is the reason ifdesc_regex exists. The
	// authors are using the following regex to keep just the actual desc:
	// "^_[a-z]{3}_[0-9]{5}_[0-9]{5}_ [A-Z0-9]+ (.*?) *( \\(.*)?$"

	router := net.IP(flowmsg.SamplerAddress).String()
	if flowmsg.InIf > 0 {
		flowmsg.SrcIfName, flowmsg.SrcIfDesc, flowmsg.SrcIfSpeed = getCachedOrQuery(router, flowmsg.InIf)
	}
	if flowmsg.OutIf > 0 {
		flowmsg.DstIfName, flowmsg.DstIfDesc, flowmsg.DstIfSpeed = getCachedOrQuery(router, flowmsg.OutIf)
	}
	return &flowmsg
}
