package server

import (
	"io"
	"log"

	pb "github.com/plan-systems/plan-vault-libp2p/protos"
	"google.golang.org/grpc"
)

var DefaultPort = int(pb.Const_DefaultGrpcServicePort)

func RegisterVaultServer(grpcServer *grpc.Server) {
	vaultSrv := &VaultServer{}
	pb.RegisterVaultGrpcServer(grpcServer, vaultSrv)
}

type VaultServer struct {
	pb.UnimplementedVaultGrpcServer
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
