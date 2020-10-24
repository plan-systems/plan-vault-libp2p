package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/plan-systems/plan-vault-libp2p/server"
)

// TODO: improve the configuration story here
var (
	tls      = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	certFile = flag.String("cert_file", "x509/server_cert.pem", "The TLS cert file")
	keyFile  = flag.String("key_file", "x509/server_key.pem", "The TLS key file")
	dataDir  = flag.String("data_dir", "", "Path to data directory")
	grpcAddr = flag.String("addr", "127.0.0.1", "gRPC server address")
	grpcPort = flag.Int("port", server.DefaultPort, "gRPC server port port")
)

func main() {
	flag.Parse()
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", *grpcAddr, *grpcPort))
	if err != nil {
		log.Fatalf("failed to set up listener: %v", err)
	}

	// TODO: should probably move all this stuff into the server
	// package and just hand it our configuration flags
	var opts []grpc.ServerOption
	if *tls {
		creds, err := credentials.NewServerTLSFromFile(*certFile, *keyFile)
		if err != nil {
			log.Fatalf("failed to generate tls creds: %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}

	grpcServer := grpc.NewServer(opts...)

	server.RegisterVaultServer(grpcServer)
	grpcServer.Serve(listener)
}
