package p2p

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/host"
	libpeer "github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	libp2pquic "github.com/libp2p/go-libp2p-quic-transport"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
	maddr "github.com/multiformats/go-multiaddr"

	"github.com/plan-systems/plan-vault-libp2p/pnode"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
	"github.com/plan-systems/plan-vault-libp2p/store"
)

type Host struct {
	host.Host
	store   *store.Store
	pubsub  *pubsub.PubSub
	ctx     context.Context
	keyring *pnode.KeyRing

	// TODO: this duplicates data stored in the host's pstorememory, do we
	// really need to store it at all?
	peers []*Peer
	lock  sync.RWMutex

	handlers map[string]*TopicHandler
}

func New(ctx context.Context, db *store.Store, cfg Config) (*Host, error) {

	if cfg.NoP2P { // standalone vault
		return nil, nil
	}

	priv, _, err := getKeys(cfg.KeyFile)
	if err != nil {
		return nil, err
	}

	// TODO: move all configuration values into Config
	tcpAddr := fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", cfg.Port)
	quicAddr := fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic", cfg.Port)

	h, err := libp2p.New(ctx,
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(tcpAddr, quicAddr),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Transport(libp2pquic.NewTransport), // TODO: keep this? probably ok?
		libp2p.DefaultTransports,
		libp2p.ConnectionManager(connmgr.NewConnManager(
			10,          // Lowwater
			100,         // HighWater,
			time.Minute, // GracePeriod
		)),
		libp2p.NATPortMap(),
	)
	if err != nil {
		return nil, err
	}

	p, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		return nil, err
	}

	host := &Host{
		Host:   h,
		store:  db,
		pubsub: p,
		peers:  []*Peer{},
	}

	if !cfg.NoMDNS {
		err = setupDiscoverymDNS(ctx, host)
		if err != nil {
			return nil, err
		}
	}

	disc, err := NewChannelDiscovery(ctx, host, &cfg)
	if err != nil {
		return nil, err
	}

	host.handlers[cfg.DiscoveryChannelURI] = disc
	go host.watchTopics()

	return host, nil
}

func (h *Host) watchTopics() {

	channels := h.store.Channels()
	for _, channel := range channels {
		go h.joinChannel(channel)
	}

	// note that we need to call watchTopics before starting the server
	// in main so that we can't race with the creation of new topics.
	//
	// TODO: this is going to make gracefully restarting the host
	// painful if we have config updates.
	updates := h.store.ChannelUpdates()
	for {
		select {
		case channel := <-updates:
			if channel != nil {
				go h.joinChannel(channel)
			}
		case <-h.ctx.Done():
			return
		}
	}
}

func (h *Host) joinChannel(channel *store.Channel) {

	h.lock.Lock()
	defer h.lock.Unlock()

	if _, ok := h.handlers[channel.URI()]; ok {
		return
	}

	th, err := NewTopicHandler(h, channel,
		func(entry *pb.Msg) error {
			_, err := channel.Append(entry)
			return err
		},
		func(*pb.Msg) error { return nil },
	)

	if err != nil {
		fmt.Println(err) // TODO: now what?!
	}
	h.handlers[channel.URI()] = th
	go th.watchTopic()
}

func (h *Host) restore(state *pb.PubsubState) error {
	if state == nil || h.pubsub == nil {
		return nil
	}
	h.lock.Lock()
	defer h.lock.Unlock()

	for _, topic := range state.Topics {
		h.pubsub.Join(topic)
	}

	h.peers = []*Peer{}
	for _, peerMsg := range state.Peers {
		peer := &Peer{}
		err := peer.decode(peerMsg)
		if err != nil {
			return err // TODO: should we try to restore the rest?
		}
		h.peers = append(h.peers, peer)
	}
	return nil
}

func (h *Host) snapshot() (*pb.PubsubState, error) {
	if h.pubsub == nil {
		return nil, nil
	}
	h.lock.RLock()
	defer h.lock.RUnlock()

	state := &pb.PubsubState{}
	state.Topics = append(state.Topics, h.pubsub.GetTopics()...)

	for _, peer := range h.peers {
		state.Peers = append(state.Peers, peer.encode())
	}
	return state, nil
}

// Peer unpacks a libp2p peer.AddrInfo struct to include the public
// key, so that we can use the public key to verify entries the peer
// signs
type Peer struct {
	AddrInfo libpeer.AddrInfo
	PubKey   []byte
}

func (peer *Peer) ID() libpeer.ID {
	return peer.AddrInfo.ID
}

func (peer *Peer) decode(peerMsg *pb.Peer) error {
	addrs := []maddr.Multiaddr{}
	for _, ma := range peerMsg.GetMultiaddrs() {
		addr, err := maddr.NewMultiaddrBytes(ma)
		if err != nil {
			return err
		}
		addrs = append(addrs, addr)
	}
	ai := libpeer.AddrInfo{
		ID:    libpeer.ID(peerMsg.GetID()),
		Addrs: addrs,
	}
	peer.PubKey = peerMsg.GetKey()
	peer.AddrInfo = ai
	return nil
}

func (peer Peer) encode() *pb.Peer {
	peerMsg := &pb.Peer{Key: peer.PubKey, ID: string(peer.ID())}
	for _, maddr := range peer.AddrInfo.Addrs {
		peerMsg.Multiaddrs = append(peerMsg.Multiaddrs, maddr.Bytes())
	}
	return peerMsg
}
