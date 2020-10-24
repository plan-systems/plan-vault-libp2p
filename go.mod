module github.com/plan-systems/plan-vault-libp2p

go 1.15

require (
	github.com/golang/protobuf v1.4.3
	github.com/libp2p/go-libp2p v0.11.0
	github.com/libp2p/go-libp2p-connmgr v0.2.4

	// note: we can't switch to v0.7.0 until go-libp2p does
	github.com/libp2p/go-libp2p-core v0.6.1
	github.com/libp2p/go-libp2p-pubsub v0.3.6
	github.com/libp2p/go-libp2p-quic-transport v0.8.2
	github.com/libp2p/go-libp2p-tls v0.1.3
	google.golang.org/grpc v1.33.1
	google.golang.org/protobuf v1.25.0
)
