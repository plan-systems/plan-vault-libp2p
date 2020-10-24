package main

import (
	"context"
	"flag"
	"log"

	"github.com/plan-systems/plan-vault-libp2p/node"
	"github.com/plan-systems/plan-vault-libp2p/server"
)

func main() {
	flag.Parse()
	ctx := context.Background()

	host, err := node.New(ctx)
	if err != nil {
		log.Fatal(err)
	}
	server.Run(ctx, host)
}
