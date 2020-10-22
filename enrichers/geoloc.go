package enrichers

import (
	flow "github.com/bwNetFlow/protobuf/go"
	maxmind "github.com/oschwald/maxminddb-golang"
	"log"
	"net"
)

var (
	geolocDb *maxmind.Reader
	dbrecord struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}
)

// Open geolocation database.
func InitGeoloc(path string) {
	var err error
	geolocDb, err = maxmind.Open(path)
	if err != nil {
		log.Println(err)
	}
}

// Close geolocation database. It's best to defer this after Init.
func CloseGeoloc() {
	geolocDb.Close()
}

// Adds Geoloc of the provided (remote) address to the flow message.
func AddGeoloc(address net.IP, flow flow.FlowMessage) *flow.FlowMessage {
	err := geolocDb.Lookup(address, &dbrecord)
	if err != nil {
		log.Println(err)
		return &flow
	}
	flow.RemoteCountry = dbrecord.Country.ISOCode
	return &flow
}
