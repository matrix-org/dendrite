package relaytest

import (
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/relay"
)

type hostFunc struct {
	ch    *tchannel.Channel
	stats *MockStats
	fn    func(relay.CallFrame, *tchannel.Connection) (string, error)
}

type hostFuncPeer struct {
	*MockCallStats

	peer *tchannel.Peer
}

// HostFunc wraps a given function to implement tchannel.RelayHost.
func HostFunc(fn func(relay.CallFrame, *tchannel.Connection) (string, error)) tchannel.RelayHost {
	return &hostFunc{nil, NewMockStats(), fn}
}

func (hf *hostFunc) SetChannel(ch *tchannel.Channel) {
	hf.ch = ch
}

func (hf *hostFunc) Start(cf relay.CallFrame, conn *tchannel.Connection) (tchannel.RelayCall, error) {
	var peer *tchannel.Peer

	peerHP, err := hf.fn(cf, conn)
	if peerHP != "" {
		peer = hf.ch.GetSubChannel(string(cf.Service())).Peers().GetOrAdd(peerHP)
	}

	// We still track stats if we failed to get a peer, so return the peer.
	return &hostFuncPeer{hf.stats.Begin(cf), peer}, err
}

func (hf *hostFunc) Stats() *MockStats {
	return hf.stats
}

func (p *hostFuncPeer) Destination() (*tchannel.Peer, bool) {
	return p.peer, p.peer != nil
}
