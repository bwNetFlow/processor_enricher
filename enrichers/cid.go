package enrichers

import (
	"encoding/csv"
	"github.com/bwNetFlow/ip_prefix_trie"
	flow "github.com/bwNetFlow/protobuf/go"
	"io"
	"log"
	"net"
	"os"
	"strconv"
)

// We have to use separate tries for IPv4 and IPv6
// TODO: maybe replace this with the newer, not homemade kentik/patricia
var TrieV4, TrieV6 ip_prefix_trie.TrieNode

// This will read the prefix list CSV and populate both Tries (v4 and v6)
func InitCid(file string) {
	f, err := os.Open(file)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	csvr := csv.NewReader(f)
	var count int
	for {
		row, err := csvr.Read()
		if err != nil {
			if err != io.EOF {
				log.Fatalf("Error opening Prefix List: %v", err)
			}
			break
		}

		cid, err := strconv.ParseInt(row[1], 10, 32)
		if err != nil {
			continue // TODO: do something useful
		}

		// copied from net.IP module to detect v4/v6
		for i := 0; i < len(row[0]); i++ {
			switch row[0][i] {
			case '.':
				TrieV4.Insert(cid, []string{row[0]})
				count += 1
				break
			case ':':
				TrieV6.Insert(cid, []string{row[0]})
				count += 1
				break
			}
		}
	}
	log.Printf("Parsed Prefix List with %d CIDs.", count)
}

// This function matches the address in the correct Trie and annotates the flow
// message with the result. It takes an address in addition to the flow message
// so it does not have to determine which address in the flow is local. This
// knowledge comes from the peerinfo enricher.
func AddCid(address net.IP, flowmsg flow.FlowMessage) *flow.FlowMessage {
	if address == nil {
		return &flowmsg
	}
	// prepare matching the address into a prefix and its associated CID
	if address.To4() == nil {
		retCid, _ := TrieV6.Lookup(address).(int64) // try to get a CID
		flowmsg.Cid = uint32(retCid)
	} else {
		retCid, _ := TrieV4.Lookup(address).(int64) // try to get a CID
		flowmsg.Cid = uint32(retCid)
	}

	/* It is possible (likely) that not all
	* prefixes are mapped to a customer id. The
	* following lines would print these unmatched
	* cases. TODO: debug logging
	 */
	// if flowmsg.Cid == 0 {
	// 	log.Printf("%v (%d -> %d, %v): CID unmatched!",
	// 		router,
	// 		flowmsg.SrcIf,
	// 		flowmsg.DstIf,
	// 		address)
	// }
	return &flowmsg
}
