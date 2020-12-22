package main

import (
	"context"

	"github.com/apex/log"
	"google.golang.org/grpc"

	vclient "github.com/plan-systems/plan-vault-libp2p/client"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
)

func main() {

	ctx := context.Background()

	client, err := vclient.New(ctx, "127.0.0.1:5091", []grpc.DialOption{})
	if err != nil {
		panic(err)
	}

	waitc := make(chan struct{})
	go func() {
		for {
			in, err := client.Recv()
			if err == vclient.ErrorStreamClosed {
				// read done.
				close(waitc)
				return
			}
			if err != nil {
				log.Fatalf("Failed to receive a message : %v", err)
			}
			log.Infof("Got message %x", in.EntryHeader.GetEntryID())
		}
	}()

	reqs := []*pb.FeedReq{}

	for _, req := range reqs {
		if err := client.Send(req); err != nil {
			log.Fatalf("Failed to send a request: %v", err)
		}
	}
	client.Close()
	<-waitc

}
