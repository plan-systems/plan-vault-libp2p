package p2p

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/plan-systems/plan-vault-libp2p/helpers"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
	"github.com/plan-systems/plan-vault-libp2p/store"
)

func NewChannelDiscovery(pctx context.Context, h *Host, cfg *Config) (*TopicHandler, error) {
	channelID := helpers.ChannelURItoChannelID(cfg.DiscoveryChannelURI)
	channel, err := h.store.Channel(channelID)
	if err != nil {
		return nil, err
	}
	th, err := NewTopicHandler(h, channel,
		func(entry *pb.Msg) error {
			return handleDiscoveryEntry(h, channel, entry)
		},
		func(entry *pb.Msg) error {
			return handleDiscoveryEntry(h, channel, entry)
		},
	)
	if err != nil {
		return nil, err
	}

	go th.watchTopic()
	return th, nil
}

func handleDiscoveryEntry(h *Host, channel *store.Channel, entry *pb.Msg) error {

	body, err := h.keyring.DecodeEntry(entry)
	if err != nil {
		return err
	}
	update := &pb.Peer{}
	err = proto.Unmarshal(body, update)
	if err != nil {
		return err
	}

	switch update.GetOp() {
	case pb.PeerUpdateOp_Upsert:
		peer := &Peer{}
		err := peer.decode(update)
		if err != nil {
			return err
		}

		// Connect ensures there is a connection between this host and
		// the peer, and store the AddrInfo into our internal peerstore.
		//
		// TODO: Connect issues a h.Network.Dial and blocks untl a
		// connection is open or an error is returned; we should
		// probably not block here?
		ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
		defer cancel()
		err = h.Connect(ctx, peer.AddrInfo)
		if err != nil {
			// TODO: we should exponentially back off but how do we get a
			// notification that the peer is live again? should we write
			// an entry that it was gone?
			fmt.Println("now what?")
			return err
		}

	case pb.PeerUpdateOp_RemovePermanently:
		peer := &Peer{}
		err := peer.decode(update)
		if err != nil {
			return err
		}
		// TODO: remove this from the p2p host
	default:
		return fmt.Errorf("unsupported operation")
	}
	return nil
}
