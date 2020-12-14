package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/apex/log"
	"github.com/plan-systems/plan-vault-libp2p/helpers"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
	"github.com/plan-systems/plan-vault-libp2p/store"
)

// TODO: figure out how we want to tune this
// note: we can't send more than 1MB in a libp2p entry
const maxBodySize = 10000000

var (
	ErrorInvalidRequest       = errors.New("invalid request")
	ErrorInvalidOpenReq       = errors.New("invalid open request")
	ErrorInvalidFeedURI       = errors.New("invalid feed URI")
	ErrorInvalidEntryBody     = errors.New("invalid entry body")
	ErrorInvalidUnsupportedOp = errors.New("unsupported operation")
)

func Run(ctx context.Context, db *store.Store, cfg *Config) {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port))
	if err != nil {
		cfg.Log.Fatalf("failed to set up listener: %v", err)
	}

	var opts []grpc.ServerOption
	if cfg.TLSCertPath != "" && cfg.TLSKeyPath != "" {
		creds, err := credentials.NewServerTLSFromFile(cfg.TLSCertPath, cfg.TLSKeyPath)
		if err != nil {
			cfg.Log.Fatalf("failed to generate tls creds: %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}

	// TODO: look thru the options to thread a context thru here
	grpcServer := grpc.NewServer(opts...)

	vaultSrv := &VaultServer{ctx: ctx, db: db, log: cfg.Log}
	pb.RegisterVaultGrpcServer(grpcServer, vaultSrv)
	grpcServer.Serve(listener)
}

type VaultServer struct {
	pb.UnimplementedVaultGrpcServer

	ctx context.Context
	db  *store.Store
	log *log.Entry
}

func (v *VaultServer) VaultSession(session pb.VaultGrpc_VaultSessionServer) error {

	ctx, cancel := context.WithCancel(v.ctx)
	conn := newConnection(session, ctx)
	defer cancel()

	for {
		select {
		case <-conn.ctx.Done():
			return nil
		default:
			req, err := conn.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			switch req.GetReqOp() {
			case pb.ReqOp_ChannelGenesis:
				err = conn.Send(v.new(req))
			case pb.ReqOp_OpenFeed:
				err = conn.Send(v.open(conn, req))
			case pb.ReqOp_CancelReq:
				err = conn.Send(v.close(conn, req))
			case pb.ReqOp_AppendEntry:
				err = conn.Send(v.append(req))
			default:
				err = conn.Send(v.unsupported(req))
			}

			if err != nil {
				v.log.Errorf("send failed: %v", err)
			}
		}
	}
}

// new writes a genesis entry for the channel
// TODO: it's not clear how this should behave any differently than `append`?
func (v *VaultServer) new(req *pb.FeedReq) *pb.Msg {
	return v.append(req)
}

// open starts a stream of entries from a Channel in the Store. A
// session can have multiple open streams.
func (v *VaultServer) open(conn *Connection, req *pb.FeedReq) *pb.Msg {

	reqID := req.GetReqID()
	openReq := req.GetOpenFeed()
	if openReq == nil {
		return v.errorResponse(reqID,
			pb.ErrCode_InvalidRequest, ErrorInvalidOpenReq)

	}
	uri := openReq.GetFeedURI()
	if uri == "" {
		return v.errorResponse(reqID,
			pb.ErrCode_InvalidFeedURI, ErrorInvalidFeedURI)
	}

	channelID := helpers.ChannelURItoChannelID(uri)
	channel, err := v.db.Channel(channelID)
	if err != nil {
		return v.errorResponse(reqID, pb.ErrCode_DatabaseError, err)
	}
	opts := newStreamOpts(req.GetOpenFeed())

	ctx, cancel := context.WithCancel(conn.ctx)
	stream := &Stream{conn: conn, id: reqID, cancel: cancel}
	conn.streams[reqID] = stream

	channel.Subscribe(ctx, stream, opts)

	resp := &pb.Msg{
		Op:    pb.MsgOp_ReqComplete,
		ReqID: reqID,
		Status: &pb.ReqStatus{
			Code: pb.StatusCode_Working,
		},
	}
	return resp

}

func newStreamOpts(req *pb.OpenFeedReq) *store.StreamOpts {

	mode := req.GetStreamMode()
	start := req.GetSeekEntryID()

	skipFirst := store.OptNone
	idsOnly := store.OptNone
	fromHead := store.OptNone
	fromGenesis := store.OptNone
	fromIndex := store.OptNone

	switch mode {
	case pb.StreamMode_DontStream:
		return nil
	case pb.StreamMode_FromGenesis:
		start = []byte{}
		fromGenesis = store.OptFromGenesis
	case pb.StreamMode_AtEntry:
		// no-op
	case pb.StreamMode_AfterEntry:
		skipFirst = store.OptSkipFirst
	case pb.StreamMode_AfterHead:
		skipFirst = store.OptSkipFirst
		fromHead = store.OptFromHead
		start = []byte{}
	case pb.StreamMode_FromIndex:
		start = []byte{}
		fromIndex = store.OptFromIndex
	}

	if req.GetSendEntryIDsOnly() {
		idsOnly = store.OptKeysOnly
	}

	var max uint64
	i := req.GetMaxEntriesToSend()
	if i >= 0 { // avoid overflow from int32
		max = uint64(i)
	}
	if max <= 0 {
		max = store.Tail
	}

	// TODO: should we reject (fromHead | fromIndex)?

	return &store.StreamOpts{
		Seek:  start,
		Max:   max,
		Flags: idsOnly | skipFirst | fromHead | fromGenesis | fromIndex,
	}

}

func newConnection(session pb.VaultGrpc_VaultSessionServer, ctx context.Context) *Connection {
	return &Connection{
		session,
		ctx,
		helpers.NewUUID(),
		map[int32]*Stream{},
		sync.RWMutex{},
	}
}

// Connection is a thin wrapper around the gRPC session, with a unique
// ID per connection so that we can keep track of connections
type Connection struct {
	pb.VaultGrpc_VaultSessionServer
	ctx     context.Context
	id      helpers.UUID
	streams map[int32]*Stream
	lock    sync.RWMutex
}

// Stream is a thin wrapper around Connection with a unique
// client-determined ID, so that clients can reference the stream
// later
type Stream struct {
	conn   *Connection
	id     int32 // client-determined ID for the stream
	cancel context.CancelFunc
}

// Publish sends the message thru the stream's connection to the
// client
func (s *Stream) Publish(msg *pb.Msg) {
	msg.ReqID = s.id
	s.conn.Send(msg)
}

// Done removes the stream from the Connection
func (s *Stream) Done() {
	s.cancel()
	s.conn.lock.Lock()
	defer s.conn.lock.Unlock()
	delete(s.conn.streams, s.id)
}

// ID returns the connection ID because the store enforces a
// per-connection unique constraint on streams. Opening a new stream
// on the same channel closes the old stream.
func (s *Stream) ID() helpers.UUID { return s.conn.id }

func (v *VaultServer) close(conn *Connection, req *pb.FeedReq) *pb.Msg {
	reqID := req.GetReqID()
	conn.lock.RLock()
	stream, ok := conn.streams[reqID]
	conn.lock.RUnlock()

	if ok {
		stream.Done()
	}

	// closing is idempotent; we don't return an error if there's no
	// stream with this ReqID because a concurrent request may have
	// done it already
	resp := &pb.Msg{
		Op:    pb.MsgOp_ReqDiscarded,
		ReqID: reqID,
		// TODO: the reqID is already in the message, so why do
		// we need it in the status?
	}
	return resp
}

// append writes a new entry to the Store
func (v *VaultServer) append(req *pb.FeedReq) *pb.Msg {
	reqID := req.GetReqID()
	entry := req.GetNewEntry()
	header := entry.GetEntryHeader()
	uri := header.GetFeedURI()
	if uri == "" {
		return v.errorResponse(reqID, pb.ErrCode_InvalidFeedURI,
			ErrorInvalidFeedURI)
	}
	body := entry.GetBody()
	if len(body) == 0 || len(body) > maxBodySize {
		return v.errorResponse(reqID, pb.ErrCode_InvalidRequest,
			ErrorInvalidEntryBody)

	}
	channelID := helpers.ChannelURItoChannelID(uri)
	channel, err := v.db.Channel(channelID)
	if err != nil {
		return v.errorResponse(reqID, pb.ErrCode_DatabaseError, err)
	}
	key, err := channel.Append(entry)
	if err != nil {
		return v.errorResponse(reqID, pb.ErrCode_DatabaseError, err)
	}

	resp := &pb.Msg{
		Op:          pb.MsgOp_ReqComplete,
		ReqID:       reqID,
		EntryHeader: &pb.EntryHeader{EntryID: key},
	}
	return resp
}

func (v *VaultServer) unsupported(req *pb.FeedReq) *pb.Msg {
	return v.errorResponse(req.GetReqID(),
		pb.ErrCode_InvalidRequest, ErrorInvalidUnsupportedOp)
}

func (v *VaultServer) errorResponse(reqID int32, code pb.ErrCode, err error) *pb.Msg {
	v.log.Error(err.Error())
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
