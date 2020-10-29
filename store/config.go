package store

import (
	"flag"
	"log"

	"github.com/dgraph-io/badger/v2"
	"github.com/libp2p/go-libp2p-core/crypto"
)

// TODO: improve the configuration story here
var (
	dataDir = flag.String("data_dir", "/tmp/badger", "directory for database storage")
)

type Config struct {
	DataDir string
	PeerID  [32]byte
	DB      badger.Options
}

func DefaultConfig() Config {

	// TODO: need to use the same keypair as the p2p PeerID
	_, pub, err := crypto.GenerateKeyPair(
		crypto.Ed25519, // Select your key type. Ed25519 are nice short
		-1,             // Select key length when possible (i.e. RSA).
	)
	if err != nil {
		log.Fatalf(err.Error())
	}
	buf, _ := pub.Raw()
	k := [32]byte{}
	copy(k[:], buf[:])

	return Config{
		//DataDir: *dataDir,
		PeerID: k,
		DB:     badger.DefaultOptions(*dataDir),
	}
}
