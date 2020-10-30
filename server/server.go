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

	"github.com/plan-systems/plan-vault-libp2p/helpers"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
	"github.com/plan-systems/plan-vault-libp2p/store"
)

// TODO: figure out how we want to tune this
const maxBodySize = 10000000

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

	// TODO: need to maintain stream state so that we stream new entries back
	s := Stream{stream, helpers.NewUUID()}
	//	s.id =

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
			err = stream.Send(v.open(s, req))
		case pb.ReqOp_CancelReq:
			err = stream.Send(v.close(s, req))
		case pb.ReqOp_AppendEntry:
			err = stream.Send(v.append(req))
		default:
			err = stream.Send(v.unsupported(req))
		}

		if err != nil {
			// TODO: do something better here
			log.Fatalf("send failed: %v", err)
		}

	}
}

// TODO: calling this the channel genesis is probably too high
// level for the Vault to understand: how could we determine
// if we're the "first" entry for the entire channel if
// another peer might already have that channel open? I think we
// can only guarantee our (channel ID + own ID + 0)
func (v *VaultServer) new(req *pb.FeedReq) *pb.Msg { return nil }

// open starts a stream of entries from a Channel in the Store. A
// session can have multiple open streams.
func (v *VaultServer) open(stream Stream, req *pb.FeedReq) *pb.Msg {

	reqID := req.GetReqID()
	openReq := req.GetOpenFeed()
	if openReq == nil {
		return errorResponse(reqID, pb.ErrCode_InvalidRequest,
			fmt.Errorf("invalid open request: missing OpenReq"))

	}
	uri := openReq.GetFeedURI()
	if uri == "" {
		return errorResponse(reqID, pb.ErrCode_InvalidFeedURI,
			fmt.Errorf("invalid feed URI"))
	}

	channelID := helpers.ChannelURItoChannelID(uri)
	channel, err := v.db.Channel(channelID)
	if err != nil {
		return errorResponse(reqID, pb.ErrCode_DatabaseError, err)
	}
	start := newStreamStart(channel, req.GetOpenFeed())
	channel.Subscribe(context.TODO(), &stream, start)

	resp := &pb.Msg{
		Op:    pb.MsgOp_ReqComplete,
		ReqID: reqID,
		Status: &pb.ReqStatus{
			Code: pb.StatusCode_Working,
		},
	}
	return resp

}

func newStreamStart(channel *store.Channel, req *pb.OpenFeedReq) *store.StreamStart {

	mode := req.GetStreamMode()
	start := req.GetSeekEntryID() // TODO: we need to convert this to a Store txnID?

	switch mode {
	case pb.StreamMode_DontStream:
		return nil
	case pb.StreamMode_FromGenesis:
		start = channel.FirstKey()
	case pb.StreamMode_AtEntry:
		// no-op
	case pb.StreamMode_AfterEntry:
		start = channel.KeyAfter(start)
	case pb.StreamMode_AfterHead:
		start = channel.LastKey()
	}

	var max uint64
	i := req.GetMaxEntriesToSend()
	if i >= 0 { // avoid overflow from int32
		max = uint64(i)
	}
	if max == 0 {
		max = store.Tail
	}

	return &store.StreamStart{
		Start:   start,
		Max:     max,
		IdsOnly: req.GetSendEntryIDsOnly(),
	}

}

type Stream struct {
	pb.VaultGrpc_VaultSessionServer
	id helpers.UUID
}

func (s *Stream) Send([]byte)      {}
func (s *Stream) Done()            {}
func (s *Stream) ID() helpers.UUID { return s.id }

func (v *VaultServer) close(stream Stream, req *pb.FeedReq) *pb.Msg { return nil }

// append writes a new entry to the Store
func (v *VaultServer) append(req *pb.FeedReq) *pb.Msg {
	reqID := req.GetReqID()
	entry := req.GetNewEntry()
	header := entry.GetEntryHeader()
	uri := header.GetFeedURI()
	if uri == "" {
		return errorResponse(reqID, pb.ErrCode_InvalidFeedURI,
			fmt.Errorf("invalid feed URI"))
	}
	body := entry.GetBody()
	if len(body) == 0 || len(body) > maxBodySize {
		return errorResponse(reqID, pb.ErrCode_InvalidRequest,
			fmt.Errorf("invalid entry body"))
	}

	channelID := helpers.ChannelURItoChannelID(uri)
	channel, err := v.db.Channel(channelID)
	if err != nil {
		return errorResponse(reqID, pb.ErrCode_DatabaseError, err)
	}
	_, err = channel.Append(body)
	if err != nil {
		return errorResponse(reqID, pb.ErrCode_DatabaseError, err)
	}

	// TODO: figure out if we need to fill out anything more than this
	resp := &pb.Msg{
		Op:    pb.MsgOp_ReqComplete,
		ReqID: reqID,
		Status: &pb.ReqStatus{
			Code: pb.StatusCode_Working,
			// EntryID: txnID,
		},
	}
	return resp
}

func (v *VaultServer) unsupported(req *pb.FeedReq) *pb.Msg {
	return errorResponse(req.GetReqID(), pb.ErrCode_InvalidRequest,
		fmt.Errorf("unsupported op: %v", req.GetReqOp()))
}

func errorResponse(reqID int32, code pb.ErrCode, err error) *pb.Msg {
	log.Printf("error: %v", err)
	resp := &pb.Msg{
		Op:    pb.MsgOp_ReqDiscarded,
		ReqID: reqID,
		Err: &pb.ReqErr{
			Code: code,
			Msg:  err.Error(),
		},
	}
	return resp

}
