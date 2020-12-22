package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	vclient "github.com/plan-systems/plan-vault-libp2p/client"
	"github.com/plan-systems/plan-vault-libp2p/keyring"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
	"github.com/plan-systems/plan-vault-libp2p/server"
)

func pollClient(m model) {
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
			msg, err := m.client.Recv()
			if err != nil {
				return
			}
			m.entryCh <- msg
		}
	}
}

// pollNextEntry for activity from the client and deliver it to the
// next Update
func pollNextEntry(m model) tea.Cmd {
	return func() tea.Msg {
		return <-m.entryCh
	}
}

func connect(ctx context.Context, cfg *server.Config) *vclient.Client {
	addr := fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port)

	var opts []grpc.DialOption

	if !cfg.Insecure && cfg.TLSCertPath != "" {
		creds, err := credentials.NewClientTLSFromFile(cfg.TLSCertPath, "")
		if err != nil {
			log.Fatalf("failed to generate tls creds: %v", err)
		}
		opts = []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	}

	client, err := vclient.New(ctx, addr, opts)
	if err != nil {
		log.Fatalf("failed to connect to plan-vault: %v", err)
	}
	return client
}

func handleEntry(kr *keyring.KeyRing, entry *pb.Msg) tea.Cmd {
	return func() tea.Msg {
		body, err := kr.DecodeEntry(entry)
		if err != nil {
			return MessageError(err)
		}

		switch entry.GetOp() {
		case pb.MsgOp_ReqComplete:
			return MessageWorkDone(struct{}{})
		case pb.MsgOp_ReqDiscarded:
			return MessageError(errors.New(string(body)))
		default:
			return MessageContent(body)
		}
	}
}
