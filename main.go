package main

import (
	"context"
	"log"
	_ "net/http/pprof"
	"os"
	"os/signal"

	"github.com/plan-systems/plan-vault-libp2p/config"
	"github.com/plan-systems/plan-vault-libp2p/metrics"
	"github.com/plan-systems/plan-vault-libp2p/p2p"
	"github.com/plan-systems/plan-vault-libp2p/server"
	"github.com/plan-systems/plan-vault-libp2p/store"
)

func main() {
	cfg := config.NewConfig()
	ctx, cancel := context.WithCancel(context.Background())

	metrics.Run(ctx, cfg.Metrics)

	db, err := store.New(ctx, cfg.StoreConfig)
	if err != nil {
		log.Fatal(err)
	}

	_, err = p2p.New(ctx, db, cfg.P2PConfig)
	if err != nil {
		log.Fatal(err)
	}
	server.Run(ctx, db, cfg.ServerConfig)

	waitForShutdown(cancel)
}

func waitForShutdown(cancel context.CancelFunc) {
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		cancel()
	}()
}
