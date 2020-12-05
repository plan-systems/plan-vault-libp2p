package main

import (
	"context"
	"flag"
	"log"

	"github.com/plan-systems/plan-vault-libp2p/p2p"
	"github.com/plan-systems/plan-vault-libp2p/server"
	"github.com/plan-systems/plan-vault-libp2p/store"
)

func main() {
	flag.Parse()
	ctx := context.Background()

	cfg := store.DefaultConfig()
	db, err := store.New(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}

	p2pCfg := p2p.DefaultConfig()
	_, err = p2p.New(ctx, db, p2pCfg)
	if err != nil {
		log.Fatal(err)
	}
	server.Run(ctx, db)
}
