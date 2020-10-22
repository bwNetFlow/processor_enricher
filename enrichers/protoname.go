package enrichers

import (
	flow "github.com/bwNetFlow/protobuf/go"
)

// TODO: make configurable
var PROTOMAP = map[uint32]string{
	1:  "ICMP",
	4:  "IPv4",
	6:  "TCP",
	17: "UDP",
	50: "ESP",
}

// Annotate a ProtoName field to the flow message.
func AddProtoName(flowmsg flow.FlowMessage) *flow.FlowMessage {
	flowmsg.ProtoName = PROTOMAP[flowmsg.Proto]
	return &flowmsg
}
