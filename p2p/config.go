package p2p

import "flag"

// TODO: improve the configuration story here
var (
	port    = flag.Int("p2p_port", 9000, "p2p server port port")
	keyFile = flag.String("key_file", "/tmp/vault.key", "path to key file")
)

type Config struct {
	NoP2P               bool
	NoMDNS              bool
	DiscoveryChannelURI string
	Port                int
	KeyFile             string
}

func DefaultConfig() Config {
	return Config{
		NoP2P:               false,
		NoMDNS:              false,
		DiscoveryChannelURI: "/TODO",
		Port:                *port,
		KeyFile:             *keyFile,
	}
}
