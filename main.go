package main

import (
	"context"
	"log"

	"github.com/plan-systems/plan-vault-libp2p/config"
	"github.com/plan-systems/plan-vault-libp2p/p2p"
	"github.com/plan-systems/plan-vault-libp2p/server"
	"github.com/plan-systems/plan-vault-libp2p/store"
)

func main() {
	cfg := config.NewConfig()
	ctx := context.Background()
	db, err := store.New(ctx, cfg.StoreConfig)
	if err != nil {
		log.Fatal(err)
	}

	_, err = p2p.New(ctx, db, cfg.P2PConfig)
	if err != nil {
		log.Fatal(err)
	}
	server.Run(ctx, db, cfg.ServerConfig)
}
