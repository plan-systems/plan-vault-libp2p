package server

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/plan-systems/plan-vault-libp2p/helpers"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
	"github.com/plan-systems/plan-vault-libp2p/store"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestServer_OpenClose(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db := dbSetup(t, ctx)

	server := &VaultServer{ctx: ctx, db: db}

	session := newMockSessionServer(ctx)
	go server.VaultSession(session)

	session.clientSend(&pb.FeedReq{
		ReqOp: pb.ReqOp_OpenFeed,
		ReqID: 7,
		OpenFeed: &pb.OpenFeedReq{
			FeedURI:          t.Name(),
			StreamMode:       pb.StreamMode_AfterHead,
			MaxEntriesToSend: -1,
			SendEntryIDsOnly: true,
		},
	})

	resp := <-session.tx
	require.Equal(pb.StatusCode_Working, resp.GetStatus().GetCode())
	require.Equal(int32(7), resp.GetReqID())

	ent1 := helpers.NewEntry(t.Name())
	ent1.EntryHeader.FeedURI = t.Name()

	session.clientSend(&pb.FeedReq{
		ReqOp:    pb.ReqOp_AppendEntry,
		ReqID:    8,
		NewEntry: ent1,
	})

	resp = <-session.tx
	require.Equal(pb.MsgOp_ReqComplete, resp.GetOp())
	require.Equal(pb.StatusCode_Working, resp.GetStatus().GetCode())
	require.Equal(int32(8), resp.GetReqID())

	// appended entry should come back for subscriber
	resp = <-session.tx
	require.Equal(pb.MsgOp_FeedEntry, resp.GetOp())
	require.Equal(pb.StatusCode_InfoMsg, resp.GetStatus().GetCode())
	require.Equal(int32(7), resp.GetReqID())
	require.Equal(id(ent1), resp.EntryHeader.GetEntryID())

	session.clientSend(&pb.FeedReq{
		ReqOp: pb.ReqOp_CancelReq,
		ReqID: 7,
	})
	resp = <-session.tx
	require.Equal(pb.MsgOp_ReqDiscarded, resp.GetOp())
	require.Equal(int32(7), resp.GetReqID())

	// duplicate close is ok
	session.clientSend(&pb.FeedReq{
		ReqOp: pb.ReqOp_CancelReq,
		ReqID: 7,
	})
	resp = <-session.tx
	require.Equal(pb.MsgOp_ReqDiscarded, resp.GetOp())
	require.Equal(int32(7), resp.GetReqID())

	// new append
	ent2 := helpers.NewEntry(t.Name())
	ent2.EntryHeader.FeedURI = t.Name()

	session.clientSend(&pb.FeedReq{
		ReqOp:    pb.ReqOp_AppendEntry,
		ReqID:    9,
		NewEntry: ent2,
	})

	resp = <-session.tx
	require.Equal(pb.MsgOp_ReqComplete, resp.GetOp())
	require.Equal(pb.StatusCode_Working, resp.GetStatus().GetCode())
	require.Equal(int32(9), resp.GetReqID())

	// TODO: this is a little janky... we can't wait on something we
	// never expect to come, so wait a little bit to make sure we
	// don't get a send for the subscriber
	timeout, cancelTimeout := context.WithTimeout(ctx, time.Millisecond*200)
	select {
	case <-timeout.Done():
		// ok
	case resp = <-session.tx:
		cancelTimeout()
		t.Fatalf("should not have received a new entry after subscriber was closed")
	}
}

func TestServer_StreamAppend(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db := dbSetup(t, ctx)

	server := &VaultServer{ctx: ctx, db: db}

	session := newMockSessionServer(ctx)
	go server.VaultSession(session)

	ent1 := helpers.NewEntry(t.Name())
	ent1.EntryHeader.FeedURI = t.Name()
	ent1.EntryHeader.ParentID = []byte("passthru") // store shouldn't touch this

	session.clientSend(&pb.FeedReq{
		ReqOp:    pb.ReqOp_AppendEntry,
		NewEntry: ent1,
	})

	resp := <-session.tx
	require.Equal(pb.MsgOp_ReqComplete, resp.GetOp())
	require.Equal(pb.StatusCode_Working, resp.GetStatus().GetCode())
	require.NotNil(id(ent1))

	channelID := helpers.ChannelURItoChannelID(t.Name())
	c, err := db.Channel(channelID)
	require.NoError(err)

	val, err := c.Get(c.EntryKey(id(ent1)))
	require.NoError(err)
	require.Equal(id(ent1), val.EntryHeader.GetEntryID())
	require.Equal("passthru", string(val.EntryHeader.GetParentID()))
	require.Equal([]byte(t.Name()), val.GetBody())
}

func TestServer_InvalidRequests(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := &VaultServer{ctx: ctx}
	session := newMockSessionServer(ctx)
	go server.VaultSession(session)

	tests := []struct {
		name     string
		req      *pb.FeedReq
		expected pb.ErrCode
	}{
		{
			name:     "nil req",
			req:      nil,
			expected: pb.ErrCode_InvalidRequest,
		},
		{
			name:     "unsupported op",
			req:      &pb.FeedReq{ReqOp: 50},
			expected: pb.ErrCode_InvalidRequest,
		},
		{
			name:     "open without feed req",
			req:      &pb.FeedReq{ReqOp: pb.ReqOp_OpenFeed, OpenFeed: nil},
			expected: pb.ErrCode_InvalidRequest,
		},
		{
			name: "open without feed uri",
			req: &pb.FeedReq{
				ReqOp: pb.ReqOp_OpenFeed,
				OpenFeed: &pb.OpenFeedReq{ // missing feed URI
					StreamMode:       pb.StreamMode_FromGenesis,
					MaxEntriesToSend: 10,
					SendEntryIDsOnly: true,
				},
			},
			expected: pb.ErrCode_InvalidFeedURI,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			session.clientSend(tc.req)
			resp := <-session.tx
			require.Equal(t, tc.expected, resp.GetErr().GetCode())
		})
	}

}

func TestServer_StreamOpts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		req      *pb.OpenFeedReq
		expected *store.StreamOpts
	}{
		{
			name:     "nil-open or open for append-only",
			req:      &pb.OpenFeedReq{StreamMode: pb.StreamMode_DontStream},
			expected: nil,
		},
		{
			name: "tail with seek",
			req: &pb.OpenFeedReq{
				StreamMode:       pb.StreamMode_AtEntry,
				MaxEntriesToSend: -1,
				SendEntryIDsOnly: false,
			},
			expected: &store.StreamOpts{
				Max:   store.Tail,
				Flags: store.OptNone},
		},
		{
			name: "tail from head",
			req: &pb.OpenFeedReq{
				StreamMode:       pb.StreamMode_AfterHead,
				MaxEntriesToSend: -1,
				SendEntryIDsOnly: false,
			},
			expected: &store.StreamOpts{
				Seek:  []byte{},
				Max:   store.Tail,
				Flags: store.OptFromHead | store.OptSkipFirst},
		},
		{
			name: "open for window with keys only",
			req: &pb.OpenFeedReq{
				StreamMode:       pb.StreamMode_AfterEntry,
				MaxEntriesToSend: 10,
				SendEntryIDsOnly: true,
			},
			expected: &store.StreamOpts{
				Max:   10,
				Flags: store.OptKeysOnly | store.OptSkipFirst},
		},
		{
			name: "open genesis with keys only",
			req: &pb.OpenFeedReq{
				StreamMode:       pb.StreamMode_FromGenesis,
				MaxEntriesToSend: 10,
				SendEntryIDsOnly: true,
			},
			expected: &store.StreamOpts{
				Seek:  []byte{},
				Max:   10,
				Flags: store.OptFromGenesis | store.OptKeysOnly},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require := require.New(t)
			tc.req.SeekEntryID = helpers.NewEntryID()
			got := newStreamOpts(tc.req)
			if tc.expected == nil {
				require.Nil(got)
			} else {
				if tc.expected.Seek != nil {
					require.Equal(tc.expected.Seek, got.Seek)
				} else {
					require.Equal(tc.req.SeekEntryID, got.Seek)
				}
				require.Equal(tc.expected.Max, got.Max)
				require.Equal(tc.expected.Flags, got.Flags)
			}
		})
	}

}

func id(msg *pb.Msg) []byte {
	return msg.EntryHeader.GetEntryID()
}

type MockSessionServer struct {
	ctx context.Context
	rx  chan *pb.FeedReq
	tx  chan *pb.Msg
}

func newMockSessionServer(ctx context.Context) *MockSessionServer {
	return &MockSessionServer{
		ctx: ctx,
		rx:  make(chan *pb.FeedReq),
		tx:  make(chan *pb.Msg, 10),
	}
}

// simulate a client RPC message
func (s *MockSessionServer) clientSend(req *pb.FeedReq) {
	s.rx <- req
}

func (s *MockSessionServer) Send(msg *pb.Msg) error {
	s.tx <- msg
	return nil
}

func (s *MockSessionServer) Recv() (*pb.FeedReq, error) {
	select {
	case req := <-s.rx:
		return req, nil
	case <-s.ctx.Done():
		return nil, io.EOF
	}
}

// we don't need these for testing, but they need to exist to
// implement the grpc.ServerStream interface
func (s *MockSessionServer) SetHeader(metadata.MD) error  { return nil }
func (s *MockSessionServer) SendHeader(metadata.MD) error { return nil }
func (s *MockSessionServer) SetTrailer(metadata.MD)       {}
func (s *MockSessionServer) Context() context.Context     { return s.ctx }
func (s *MockSessionServer) SendMsg(m interface{}) error  { return nil }
func (s *MockSessionServer) RecvMsg(m interface{}) error  { return nil }

func dbSetup(t *testing.T, ctx context.Context) *store.Store {
	cfg := testConfig()
	store, err := store.New(ctx, cfg)
	if err != nil {
		t.Fatalf(err.Error())
	}
	return store
}

func testConfig() store.Config {
	cfg := store.DefaultConfig()
	cfg.DB = cfg.DB.
		WithDir("").                     // need to unset for in-memory
		WithValueDir("").                // need to unset for in-memory
		WithInMemory(true).              // no cleanup
		WithLoggingLevel(badger.WARNING) // avoid test noise
	return cfg
}
