package p2p

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	libpeer "github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	libp2pquic "github.com/libp2p/go-libp2p-quic-transport"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
	maddr "github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"

	"github.com/plan-systems/plan-vault-libp2p/keyring"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
	"github.com/plan-systems/plan-vault-libp2p/store"
)

var snapshotInterval = time.Second * 30 // TODO: make this configurable
var snapshotKey = []byte{0x51, 0x90}    // magic prefix TODO: make this configurable

type Host struct {
	host.Host
	store        *store.Store
	pubsub       *pubsub.PubSub
	keyring      *keyring.KeyRing
	handlers     map[string]*TopicHandler
	discoveryURI string

	ctx              context.Context
	lock             sync.RWMutex
	snapshotUpdateCh chan struct{}
	readyCh          chan error
}

func New(ctx context.Context, db *store.Store, cfg Config) (*Host, error) {

	if cfg.NoP2P { // standalone vault
		return nil, nil
	}

	kr, err := keyring.NewFromFile(cfg.DiscoveryChannelURI, cfg.KeyFile)
	if err != nil {
		return nil, err
	}
	priv := kr.GetIdentityKey()

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
		Host:             h,
		store:            db,
		pubsub:           p,
		keyring:          kr,
		handlers:         map[string]*TopicHandler{},
		discoveryURI:     cfg.DiscoveryChannelURI,
		ctx:              ctx,
		snapshotUpdateCh: make(chan struct{}),
		readyCh:          make(chan error),
	}

	if !cfg.NoMDNS {
		err = setupDiscoverymDNS(ctx, host)
		if err != nil {
			return nil, err
		}
	}

	disc, err := NewChannelDiscovery(ctx, host, &cfg)
	if err != nil {
		return nil, fmt.Errorf("could not configure channel discovery: %w", err)
	}

	host.handlers[cfg.DiscoveryChannelURI] = disc
	go host.watchTopics()

	err = <-host.readyCh
	if err != nil {
		return nil, fmt.Errorf("could not start p2p topic monitoring: %w", err)
	}

	return host, nil
}

func (h *Host) watchTopics() {

	err := h.restore()
	if err != nil {
		h.readyCh <- fmt.Errorf("failed to restore from disk: %w", err)
	}

	// note that we need to call watchTopics before starting the server
	// in main so that we can't race with the creation of new topics.
	//
	// TODO: this is going to make gracefully restarting the host
	// painful if we have config updates.
	updates := h.store.ChannelUpdates()
	ticker := time.NewTicker(snapshotInterval)
	defer ticker.Stop()

	var dirty bool

	h.readyCh <- nil

	for {
		select {
		case channel := <-updates:
			if channel != nil {
				go h.joinChannel(channel, []byte{})
			}
		case <-h.snapshotUpdateCh:
			// note: because we snapshot in the same select, this is
			// going to block updates to the topicHandler's
			// lastEntrySeen, but that should also prevent us from
			// having to lock all the topicHandlers at the same time
			// TODO: need to test this to verify
			dirty = true
		case <-ticker.C:
			err := h.snapshot(dirty)
			if err == nil {
				dirty = false
			}
		case <-h.ctx.Done():
			return
		}
	}
}

func (h *Host) joinChannel(channel *store.Channel, start []byte) {

	h.lock.Lock()
	defer h.lock.Unlock()

	if _, ok := h.handlers[channel.URI()]; ok {
		return
	}

	th, err := NewTopicHandler(h, channel, start,
		func(entry *pb.Msg) error {
			_, err := channel.Append(entry)
			return err
		},
		func(*pb.Msg) error {
			h.snapshotUpdateCh <- struct{}{}
			return nil
		},
	)

	if err != nil {
		// TODO: this may be "topic already exists", but how should
		// we handle it when it's not?
		return
	}
	h.handlers[channel.URI()] = th
	h.snapshotUpdateCh <- struct{}{}
	go th.watchTopic()
}

// snapshot syncs our state to the Store. The caller must RLock the host.
func (h *Host) snapshot(dirty bool) error {
	if !dirty {
		return nil
	}
	h.lock.RLock()
	defer h.lock.RUnlock()
	snap, err := h.takeSnapshot()
	if err != nil {
		return err
	}
	return h.writeSnapshot(snap)
}

func (h *Host) takeSnapshot() (*pb.PubsubState, error) {
	state := &pb.PubsubState{}

	for uri, handler := range h.handlers {
		tPtr := &pb.TopicPointer{
			ID:                 uri,
			LastEntryPublished: handler.lastEntryID,
		}
		state.Topics = append(state.Topics, tPtr)
	}

	peerstore := h.Peerstore()
	for _, peerID := range peerstore.Peers() {
		ma := peerstore.Addrs(peerID)
		pubkey := peerstore.PubKey(peerID)
		peer, err := encodePeer(peerID, pubkey, ma)
		if err != nil {
			return nil, err
		}
		state.Peers = append(state.Peers, peer)
	}
	return state, nil
}

func (h *Host) writeSnapshot(snap *pb.PubsubState) error {
	body, err := proto.Marshal(snap)
	if err != nil {
		return err
	}
	entry := &pb.Msg{Body: body}
	return h.store.Upsert(snapshotKey, entry)
}

func (h *Host) restore() error {

	snap, err := h.readSnapshot()
	if err != nil {
		return err
	}

	disc := h.handlers[h.discoveryURI]
	if disc == nil {
		return fmt.Errorf("no discovery channel found")
	}
	for _, peerMsg := range snap.Peers {
		// trigger all the same operations as receiving an entry on
		// the discovery channel but bypass the cost of serializing
		// the entry
		err := handleDiscoveryUpsert(h, peerMsg)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *Host) readSnapshot() (*pb.PubsubState, error) {
	snap := &pb.PubsubState{}
	entry, err := h.store.Get(snapshotKey)
	if err != nil {
		if errors.Is(err, store.ErrorKeyNotFound) {
			return snap, nil // no previous snapshot
		} else {
			return nil, err
		}
	}

	err = proto.Unmarshal(entry.GetBody(), snap)
	if err != nil {
		return nil, err
	}
	return snap, nil
}

func encodePeer(peerID libpeer.ID, pubkey crypto.PubKey, ma []maddr.Multiaddr) (*pb.Peer, error) {

	b, err := crypto.MarshalPublicKey(pubkey)
	if err != nil {
		return nil, err
	}

	peerMsg := &pb.Peer{Op: pb.PeerUpdateOp_Upsert, Key: b, ID: string(peerID)}
	for _, maddr := range ma {
		peerMsg.Multiaddrs = append(peerMsg.Multiaddrs, maddr.Bytes())
	}
	return peerMsg, nil
}

func decodePeerAddr(encoded *pb.Peer) (libpeer.AddrInfo, error) {

	ai := libpeer.AddrInfo{
		ID:    libpeer.ID(encoded.GetID()),
		Addrs: []maddr.Multiaddr{},
	}

	for _, b := range encoded.Multiaddrs {
		ma, err := maddr.NewMultiaddrBytes(b)
		if err != nil {
			return ai, err
		}
		ai.Addrs = append(ai.Addrs, ma)
	}
	return ai, nil
}
