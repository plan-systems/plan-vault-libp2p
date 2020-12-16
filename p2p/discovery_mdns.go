package p2p

// This is a minimal mDNS discovery stub, lifted with few
// modifications from the libp2p-examples chat example. We support
// mDNS to bootstrap testing and local communities on the same LAN,
// but this isn't our typical use case.

import (
	"context"
	"time"

	"github.com/apex/log"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery"
)

// mDNSInterval is how often we republish our mDNS records
const mDNSInterval = time.Minute * 10

// mDNSServiceTag is used in our mDNS advertisements to discover other
// chat peers on the LAN
const mDNSServiceTag = "plan-vault"

// mDNSNotifee gets notified when we find a new peer via mDNS discovery
type mDNSNotifee struct {
	h      *Host
	logger *log.Entry
}

// HandlePeerFound connects to peers discovered via mDNS. Once they're
// connected, the PubSub system will automatically start interacting
// with them if they also support PubSub.
func (n *mDNSNotifee) HandlePeerFound(pi peer.AddrInfo) {
	ctx, cancel := context.WithTimeout(n.h.ctx, 10*time.Second)
	defer cancel()
	err := n.h.Connect(ctx, pi)
	if err != nil {
		n.logger.
			WithFields(log.Fields{"peer": pi.ID.Pretty()}).
			WithError(err).Error("failed to connect to peer")
	}
}

// setupDiscoverymDNS creates an mDNS discovery service and attaches
// it to the libp2p Host.
func setupDiscoverymDNS(ctx context.Context, h *Host) error {
	disc, err := discovery.NewMdnsService(ctx, h, mDNSInterval, mDNSServiceTag)
	if err != nil {
		return err
	}

	n := mDNSNotifee{
		h:      h,
		logger: h.log.WithFields(log.Fields{"service": "mDNS"}),
	}
	disc.RegisterNotifee(&n)
	return nil
}
