package p2p

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	libp2pquic "github.com/libp2p/go-libp2p-quic-transport"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
)

type Node struct {
	host   *host.Host
	pubsub *pubsub.PubSub
	ctx    context.Context
}

func New(ctx context.Context) (*Node, error) {

	// TODO: store these on disk somewheres
	priv, _, err := crypto.GenerateKeyPair(
		crypto.Ed25519, // Select your key type. Ed25519 are nice short
		-1,             // Select key length when possible (i.e. RSA).
	)
	if err != nil {
		panic(err)
	}

	// TODO: move this into our configuration setup
	tcpAddr := fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *port)
	quicAddr := fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic", *port)

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
		return nil, err // TODO: probably should panic?
	}

	// TODO: we need to actually do something with this pubsub
	p, err := pubsub.NewGossipSub(ctx, h)
	if err != nil {
		panic(err)
	}

	// TODO: we need to implement our own discovery
	err = setupDiscovery(ctx, h)
	if err != nil {
		panic(err)
	}

	return &Node{
		host:   &h,
		pubsub: p,
	}, nil

}
