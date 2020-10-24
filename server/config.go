package server

import (
	"flag"

	pb "github.com/plan-systems/plan-vault-libp2p/protos"
)

var defaultPort = int(pb.Const_DefaultGrpcServicePort)

// TODO: improve the configuration story here
var (
	tls      = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	certFile = flag.String("cert_file", "x509/server_cert.pem", "The TLS cert file")
	keyFile  = flag.String("key_file", "x509/server_key.pem", "The TLS key file")
	dataDir  = flag.String("data_dir", "", "Path to data directory")
	grpcAddr = flag.String("addr", "127.0.0.1", "gRPC server address")
	grpcPort = flag.Int("port", defaultPort, "gRPC server port port")
)
