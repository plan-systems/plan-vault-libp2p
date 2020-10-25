package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"

	"github.com/libp2p/go-libp2p-core/host"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "github.com/plan-systems/plan-vault-libp2p/protos"
	"github.com/plan-systems/plan-vault-libp2p/store"
)

func Run(ctx context.Context, nodeHost *host.Host, db *store.Store) {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", *grpcAddr, *grpcPort))
	if err != nil {
		log.Fatalf("failed to set up listener: %v", err)
	}

	var opts []grpc.ServerOption
	if *tls {
		creds, err := credentials.NewServerTLSFromFile(*certFile, *keyFile)
		if err != nil {
			log.Fatalf("failed to generate tls creds: %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}

	// TODO: look thru the options to thread a context thru here
	grpcServer := grpc.NewServer(opts...)

	vaultSrv := &VaultServer{nodeHost: nodeHost, db: db}
	pb.RegisterVaultGrpcServer(grpcServer, vaultSrv)
	grpcServer.Serve(listener)
}

type VaultServer struct {
	pb.UnimplementedVaultGrpcServer

	nodeHost *host.Host
	db       *store.Store
}

func (v *VaultServer) VaultSession(stream pb.VaultGrpc_VaultSessionServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			// TODO: make sure we've cleaned up gracefully
			return nil
		}
		if err != nil {
			// TODO: what are we supposed to be doing here?
			return err
		}
		switch req.GetReqOp() {
		case pb.ReqOp_ChannelGenesis:
			err = stream.Send(v.new(req))
		case pb.ReqOp_OpenFeed:
			err = stream.Send(v.open(req))
		case pb.ReqOp_CancelReq:
			err = stream.Send(v.close(req))
		case pb.ReqOp_AppendEntry:
			err = stream.Send(v.append(req))
		default:
			err = stream.Send(v.unsupported(req))
		}

		if err != nil {
			// TODO: do something better here
			log.Fatalf("send failed:", err)
		}

	}
	return nil
}

func (v *VaultServer) new(req *pb.FeedReq) *pb.Msg { return nil }

func (v *VaultServer) open(req *pb.FeedReq) *pb.Msg { return nil }

func (v *VaultServer) close(req *pb.FeedReq) *pb.Msg { return nil }

func (v *VaultServer) append(req *pb.FeedReq) *pb.Msg { return nil }

func (v *VaultServer) unsupported(req *pb.FeedReq) *pb.Msg {
	log.Printf("unsupported op: %v", req.GetReqOp())

	resp := &pb.Msg{
		Op:          pb.MsgOp_ReqDiscarded,
		ReqID:       req.GetReqID(),
		EntryHeader: &pb.EntryHeader{},
		Body:        []byte{},
	}
	return resp
}
