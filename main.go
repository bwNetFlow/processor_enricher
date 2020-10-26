package main

import (
	"flag"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	kafka "github.com/bwNetFlow/kafkaconnector"
	"github.com/bwNetFlow/processor_enricher/enrichers"
)

var (
	logFile = flag.String("log", "./processor_enricher.log", "Location of the log file.")

	// Kafka Connection Parameters
	kafkaBroker        = flag.String("kafka.brokers", "127.0.0.1:9092,[::1]:9092", "Kafka brokers list separated by commas")
	kafkaConsumerGroup = flag.String("kafka.consumer_group", "enricher-debug", "Kafka Consumer Group")
	kafkaInTopic       = flag.String("kafka.in.topic", "flow-messages", "Kafka topic to consume from")
	kafkaOutTopic      = flag.String("kafka.out.topic", "flows-messages-enriched", "Kafka topic to produce to")

	kafkaUser        = flag.String("kafka.user", "", "Kafka username to authenticate with")
	kafkaPass        = flag.String("kafka.pass", "", "Kafka password to authenticate with")
	kafkaAuthAnon    = flag.Bool("kafka.auth_anon", false, "Set Kafka Auth Anon")
	kafkaDisableTLS  = flag.Bool("kafka.disable_tls", false, "Whether to use tls or not")
	kafkaDisableAuth = flag.Bool("kafka.disable_auth", false, "Whether to use auth or not")

	// Cid Enricher. Adds Cid, needs local address
	addCid = flag.Bool("output.cid", false, "Whether to populate the Cid Field.")
	cidDb  = flag.String("config.cid_db", "config/example_cid_db.csv", "Location of the CID 'database', in CSV format.")

	// GeoLoc Enricher. Adds RemoteCountry, needs remote address
	addGeoLoc = flag.Bool("output.geoloc", false, "Whether to populate the RemoteCC Field.")
	geoLocDB  = flag.String("config.geoloc", "config/geolite2/GeoLite2-Country.mmdb", "Location of the GeoLite2 mmdb file.")

	// SNMPIface Enricher. Adds interface information from SNMP, needs open firewalls on the sampling routers
	// TODO: configurable set of snmp information retrived
	addSnmp         = flag.Bool("output.snmp", false, "Whether to get interface info via SNMP.")
	snmpCommunity   = flag.String("config.snmp.community", "public", "The Community used when connecting via SNMP.")
	snmpIfDescRegex = flag.String("config.snmp.ifdescregex", "(.*)", "The RegEx used to truncate the interface description.")

	// Simple Enrichers. Adding stuff without any dependency
	// TODO: configurable default to be used if the flow does not contain the sampling rate
	addNormalize = flag.Bool("output.normalize", false, "Whether to normalize all size info according to sampling rate.")
	// TODO: configurable set of 'well known' protocols which get a name
	addProtoName = flag.Bool("output.protoname", true, "Whether to populate the ProtoName Field.")
)

func main() {
	flag.Parse()
	var err error

	// initialize logger
	logfile, err := os.OpenFile(*logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		println("Error opening file for logging: %v", err)
		return
	}
	defer logfile.Close()
	mw := io.MultiWriter(os.Stdout, logfile)
	log.SetOutput(mw)
	log.Println("-------------------------- Started.")

	// trap SIGINT to trigger a shutdown.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	if *addCid {
		enrichers.InitCid(*cidDb)
	}
	if *addGeoLoc {
		enrichers.InitGeoloc(*geoLocDB)
		defer enrichers.CloseGeoloc()
	}
	if *addSnmp {
		enrichers.InitSnmp(*snmpIfDescRegex, *snmpCommunity)
	}

	// connect to the Kafka cluster
	var kafkaConn = kafka.Connector{}

	if *kafkaDisableTLS {
		log.Println("kafkaDisableTLS ...")
		kafkaConn.DisableTLS()
	}
	if *kafkaDisableAuth {
		log.Println("kafkaDisableAuth ...")
		kafkaConn.DisableAuth()
	} else { // set Kafka auth
		if *kafkaAuthAnon {
			kafkaConn.SetAuthAnon()
		} else if *kafkaUser != "" {
			kafkaConn.SetAuth(*kafkaUser, *kafkaPass)
		} else {
			log.Println("No explicit credentials available, trying env.")
			err = kafkaConn.SetAuthFromEnv()
			if err != nil {
				log.Println("No credentials available, using 'anon:anon'.")
				kafkaConn.SetAuthAnon()
			}
		}
	}

	err = kafkaConn.StartConsumer(*kafkaBroker, []string{*kafkaInTopic}, *kafkaConsumerGroup, -1) // offset -1 is the most recent flow
	if err != nil {
		log.Println("StartConsumer:", err)
		// sleep to make auto restart not too fast and spamming connection retries
		time.Sleep(5 * time.Second)
		return
	}
	err = kafkaConn.StartProducer(*kafkaBroker)
	if err != nil {
		log.Println("StartProducer:", err)
		// sleep to make auto restart not too fast and spamming connection retries
		time.Sleep(5 * time.Second)
		return
	}
	defer kafkaConn.Close()

	// receive flows in a loop
	for {
		select {
		case flowmsg, ok := <-kafkaConn.ConsumerChannel():
			if !ok {
				log.Println("Enricher ConsumerChannel closed. Investigate this!")
				return
			}
			// determine local and remote address assumes the flow exporting
			// interface ("observation point") is a peering/border interface
			// TODO: make location of exporters configurable, dont assume border
			var laddress, raddress net.IP
			switch {
			case flowmsg.FlowDirection == 0: // 0 is ingress
				laddress, raddress = flowmsg.DstAddr, flowmsg.SrcAddr
			case flowmsg.FlowDirection == 1: // 1 is egress
				laddress, raddress = flowmsg.SrcAddr, flowmsg.DstAddr
			}

			// add the cid of the local address
			if *addCid && laddress != nil {
				flowmsg = enrichers.AddCid(laddress, *flowmsg)
			}

			// add the geoloc of the remote address
			if *addGeoLoc && raddress != nil {
				flowmsg = enrichers.AddGeoloc(raddress, *flowmsg)
			}

			// add the protocol name
			if *addProtoName {
				flowmsg = enrichers.AddProtoName(*flowmsg)
			}

			// normalize fields with their sampling rate
			if *addNormalize {
				flowmsg = enrichers.AddNormalize(*flowmsg)
			}

			// add the interface descriptions via SNMP
			if *addSnmp {
				flowmsg = enrichers.AddSnmp(*flowmsg)
			}

			//send to out topic
			kafkaConn.ProducerChannel(*kafkaOutTopic) <- flowmsg
		case <-signals:
			return
		}
	}

}
