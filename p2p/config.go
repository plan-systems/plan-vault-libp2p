package p2p

import "flag"

// TODO: improve the configuration story here
var (
	port = flag.Int("p2p_port", 9000, "p2p server port port")
)
