package enrichers

import (
	flow "github.com/bwNetFlow/protobuf/go"
)

// Normalize Bytes, Packets and set the Normalized field. It is best if
// InitIfaces has been called to provide an additional source of sampling
// rates, but it is not required.
func AddNormalize(flowmsg flow.FlowMessage) *flow.FlowMessage {
	// it's easy if the flow includes the info, just multiply
	if flowmsg.SamplingRate > 0 {
		flowmsg.Bytes *= flowmsg.SamplingRate
		flowmsg.Packets *= flowmsg.SamplingRate
		flowmsg.Normalized = 1
		return &flowmsg
	}

	// still here? use 32 and be done with it
	// TODO: make configurable
	flowmsg.Bytes *= 32
	flowmsg.Packets *= 32
	flowmsg.SamplingRate = 32
	flowmsg.Normalized = 1
	return &flowmsg
}
