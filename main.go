package main

import (
	"context"
	"flag"
	"log"

	"github.com/plan-systems/plan-vault-libp2p/p2p"
	"github.com/plan-systems/plan-vault-libp2p/server"
)

func main() {
	flag.Parse()
	ctx := context.Background()

	host, err := p2p.New(ctx)
	if err != nil {
		log.Fatal(err)
	}
	server.Run(ctx, host)
}
