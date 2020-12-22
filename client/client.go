package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"google.golang.org/grpc"

	pb "github.com/plan-systems/plan-vault-libp2p/protos"
)

var ErrorStreamClosed = errors.New("stream closed by server")

type Client struct {
	ctx      context.Context
	cancelFn context.CancelFunc
	session  pb.VaultGrpc_VaultSessionClient

	// request state tracking
	reqMapping map[string]int32
	counter    int32
	lock       sync.Mutex
}

func New(pctx context.Context, addr string, opts []grpc.DialOption) (*Client, error) {
	conn, err := grpc.Dial(addr, opts...)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(pctx)

	go func() { <-ctx.Done(); conn.Close() }()
	grpcClient := pb.NewVaultGrpcClient(conn)
	session, err := grpcClient.VaultSession(ctx)
	if err != nil {
		cancel()
		return nil, err
	}

	client := &Client{
		ctx:        ctx,
		cancelFn:   cancel,
		session:    session,
		reqMapping: map[string]int32{},
	}
	return client, nil
}

// Close disconnects the client and disposes of all resources
func (c *Client) Close() {
	c.session.CloseSend()
	c.cancelFn()
}

// Recv is a blocking receive of the next message from the vault
// session. If the stream is closed by the server, will automatically
// cancel the client context and will return ErrorStreamClosed
func (c *Client) Recv() (*pb.Msg, error) {
	msg, err := c.session.Recv()
	if err == io.EOF {
		c.cancelFn()
		return nil, ErrorStreamClosed
	}
	return msg, err
}

// Send is a blocking send to the vault session.
func (c *Client) Send(req *pb.FeedReq) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.counter += 1
	req.ReqID = c.counter
	return c.sendImpl(req)
}

// OpenURI sends an OpenFeed message and tracks the URI-to-requestID
// mapping required to close it later
func (c *Client) OpenURI(uri string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.counter += 1
	c.reqMapping[uri] = c.counter
	return c.sendImpl(&pb.FeedReq{
		ReqOp: pb.ReqOp_OpenFeed,
		ReqID: c.counter,
		OpenFeed: &pb.OpenFeedReq{
			FeedURI:          uri,
			StreamMode:       pb.StreamMode_AfterHead,
			MaxEntriesToSend: -1,
			SendEntryIDsOnly: false,
		},
	})
}

// CloseURI sends an CloseFeed message using the previously tracked req ID
func (c *Client) CloseURI(uri string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	reqID, ok := c.reqMapping[uri]
	if !ok {
		return fmt.Errorf("URI %q was not open", uri)
	}
	return c.sendImpl(&pb.FeedReq{
		ReqOp: pb.ReqOp_CancelReq,
		ReqID: reqID,
	})
}

func (c *Client) sendImpl(req *pb.FeedReq) error {
	return c.session.Send(req)
}
