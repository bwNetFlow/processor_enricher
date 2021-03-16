package enrichers

import (
	"net"
	"sync"

	bwmessage "github.com/bwNetFlow/protobuf/go"
)

// CombinedEnricher combines the different enrichers into one.
type CombinedEnricher struct {
	// add the cid of the local address.
	AddCID bool
	// Location of the CID 'database', in CSV format.
	CIDDb string

	// add the geoloc of the remote address.
	AddGeoLoc bool
	// Location of the GeoLite2 mmdb file.
	GeoLocDb string

	// add the protocol name.
	AddProtoName bool

	// normalize fields with their sampling rate.
	AddNormalize bool

	// add the interface descriptions via SNMP.
	AddSNMP bool
	// The Community used when connecting via SNMP.
	SNMPCommunity string
	// The RegEx used to truncate the interface description.
	SNMPIfDescRegex string

	// Ensures the enricher is only initialized Once
	initializeOnce sync.Once
}

// Initialize the enricher. This is safe to call multiple times.
func (e *CombinedEnricher) Initialize() {
	e.initializeOnce.Do(func() {
		if e.AddCID {
			InitCid(e.CIDDb)
		}
		if e.AddGeoLoc {
			InitGeoloc(e.GeoLocDb)
			defer CloseGeoloc()
		}
		if e.AddSNMP {
			InitSnmp(e.SNMPIfDescRegex, e.SNMPCommunity)
		}
	})
}

func (e *CombinedEnricher) Process(msg *bwmessage.FlowMessage) *bwmessage.FlowMessage {
	// Ensure enricher is initialized.
	e.Initialize()

	// determine local and remote address assumes the flow exporting
	// interface ("observation point") is a peering/border interface
	// TODO: make location of exporters configurable, dont assume border
	var laddress, raddress net.IP
	switch {
	case msg.FlowDirection == 0: // 0 is ingress
		laddress, raddress = msg.DstAddr, msg.SrcAddr
	case msg.FlowDirection == 1: // 1 is egress
		laddress, raddress = msg.SrcAddr, msg.DstAddr
	}

	// add the cid of the local address
	if e.AddCID && laddress != nil {
		msg = AddCid(laddress, *msg)
	}

	// add the geoloc of the remote address
	if e.AddGeoLoc && raddress != nil {
		msg = AddGeoloc(raddress, *msg)
	}

	// add the protocol name
	if e.AddProtoName {
		msg = AddProtoName(*msg)
	}

	// normalize fields with their sampling rate
	if e.AddNormalize {
		msg = AddNormalize(*msg)
	}

	// add the interface descriptions via SNMP
	if e.AddSNMP {
		msg = AddSnmp(*msg)
	}

	return msg
}
