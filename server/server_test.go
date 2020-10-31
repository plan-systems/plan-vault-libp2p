package server

import (
	"context"
	"io"
	"testing"

	"github.com/dgraph-io/badger/v2"
	"github.com/plan-systems/plan-vault-libp2p/helpers"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
	"github.com/plan-systems/plan-vault-libp2p/store"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestServer_StreamAppend(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := testConfig()
	db, err := store.New(ctx, cfg)
	if err != nil {
		t.Fatalf(err.Error())
	}

	server := &VaultServer{db: db}

	session := newMockSessionServer(ctx)
	go server.VaultSession(session)

	session.clientSend(&pb.FeedReq{
		ReqOp: pb.ReqOp_AppendEntry,
		NewEntry: &pb.Msg{
			EntryHeader: &pb.EntryHeader{
				FeedURI:  t.Name(),
				ParentID: []byte("I am a useless piece of data"), // TODO: cut this
			},
			Body: []byte{0xFF},
		},
	})

	resp := <-session.tx
	require.Equal(pb.StatusCode_Working, resp.GetStatus().GetCode())
	key := resp.GetEntryHeader().GetEntryID()
	require.NotNil(key)

	channelID := helpers.ChannelURItoChannelID(t.Name())
	c, err := db.Channel(channelID)
	require.NoError(err)

	val, err := c.Get(key)
	require.NoError(err)
	require.Equal([]byte{0xFF}, val)

}

func TestServer_InvalidRequests(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := &VaultServer{}
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
			require.Equal(tc.expected, resp.GetErr().GetCode())
		})
	}

}

func TestServer_StreamStart(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	// TODO: it would be nice if we could have a more minimal channel
	// key interface to pass along here instead of having to stand up the
	// whole store
	c, _, cancel := dbSetup(t)
	defer cancel()

	tests := []struct {
		name     string
		req      *pb.OpenFeedReq
		expected *store.StreamStart
	}{
		{
			name:     "nil-open / open for append-only",
			req:      &pb.OpenFeedReq{StreamMode: pb.StreamMode_DontStream},
			expected: nil,
		},
		{
			name: "tail with seek",
			req: &pb.OpenFeedReq{
				StreamMode:       pb.StreamMode_AtEntry,
				SeekEntryID:      c.KeyFor(1),
				MaxEntriesToSend: -1,
				SendEntryIDsOnly: false,
			},
			expected: &store.StreamStart{
				Start: c.KeyFor(1), Max: store.Tail, IdsOnly: false},
		},
		{
			name: "tail from head",
			req: &pb.OpenFeedReq{
				StreamMode:       pb.StreamMode_AfterHead,
				SeekEntryID:      c.KeyFor(42), // should be ignored
				MaxEntriesToSend: -1,
				SendEntryIDsOnly: false,
			},
			expected: &store.StreamStart{
				Start: c.KeyFor(0), Max: store.Tail, IdsOnly: false},
		},
		{
			name: "open for window, keys only",
			req: &pb.OpenFeedReq{
				StreamMode:       pb.StreamMode_AfterEntry,
				SeekEntryID:      c.KeyFor(20),
				MaxEntriesToSend: 10,
				SendEntryIDsOnly: true,
			},
			expected: &store.StreamStart{
				Start: c.KeyFor(21), Max: 10, IdsOnly: true},
		},
		{
			name: "open genesis",
			req: &pb.OpenFeedReq{
				StreamMode:       pb.StreamMode_FromGenesis,
				SeekEntryID:      c.KeyFor(20), // should be ignored
				MaxEntriesToSend: 10,
				SendEntryIDsOnly: true,
			},
			expected: &store.StreamStart{
				Start: c.FirstKey(), Max: 10, IdsOnly: true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := newStreamStart(c, tc.req)
			if tc.expected == nil {
				require.Nil(got)
			} else {
				require.Equal(tc.expected.Start, got.Start)
				require.Equal(tc.expected.Max, got.Max)
				require.Equal(tc.expected.IdsOnly, got.IdsOnly)
			}
		})
	}

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

func dbSetup(t *testing.T) (*store.Channel, context.Context, func()) {

	ctx, cancel := context.WithCancel(context.Background())
	cfg := testConfig()
	store, err := store.New(ctx, cfg)
	if err != nil {
		t.Fatalf(err.Error())
	}

	channelID := helpers.ChannelURItoChannelID(t.Name())
	channel, err := store.Channel(channelID)
	if err != nil {
		t.Fatalf(err.Error())
	}

	return channel, ctx, cancel
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
