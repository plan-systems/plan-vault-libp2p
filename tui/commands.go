package main

import (
	"errors"
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"

	vclient "github.com/plan-systems/plan-vault-libp2p/client"
	"github.com/plan-systems/plan-vault-libp2p/keyring"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
)

// pollNextEntry blocks on entries from the vault client and delivers
// them as messages for the next Update
func pollNextEntry(m model) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
			return nil
		default:
			msg, err := m.client.Recv()
			if err != nil {
				return MessageError(err)
			}
			return msg
		}
	}
}

// connect opens a gRPC connection to the Vault described in the
// server configuration.
func connect(m model) tea.Cmd {
	return func() tea.Msg {
		cfg := m.cfg.ServerConfig
		addr := fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port)

		var opts []grpc.DialOption

		if !cfg.Insecure && cfg.TLSCertPath != "" {
			creds, err := credentials.NewClientTLSFromFile(cfg.TLSCertPath, "")
			if err != nil {
				log.Fatalf("failed to generate tls creds: %v", err)
			}
			opts = []grpc.DialOption{grpc.WithTransportCredentials(creds)}
		}

		client, err := vclient.New(m.ctx, addr, opts)
		if err != nil {
			// just bail out here
			log.Fatalf("failed to connect to plan-vault: %v", err)
		}
		return MessageConnected(client)
	}
}

// handleEntry decodes a new Vault entry and turns it into either
// progress/error message or new content
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

// handleOpenURI opens a new polling feed request to the Vault
func handleOpenURI(m model, msg MessageOpenURI) tea.Cmd {
	return func() tea.Msg {
		err := validateNonEmpty([][]string{{msg.uri, "missing channel URI"}})
		if err != nil {
			return MessageError(err)
		}
		err = m.client.OpenURI(msg.uri)
		if err != nil {
			return MessageError(err)
		}
		return MessageWorkDone{}
	}
}

// handleOpenURI closes an existing polling feed request to the Vault
func handleCloseURI(m model, msg MessageCloseURI) tea.Cmd {
	return func() tea.Msg {
		err := validateNonEmpty([][]string{{msg.uri, "missing channel URI"}})
		if err != nil {
			return MessageError(err)
		}
		err = m.client.CloseURI(msg.uri)
		if err != nil {
			return MessageError(err)
		}
		return MessageWorkDone{}
	}
}

// handleSendMessage appends a new entry on the channel
func handleSendMessage(m model, msg MessageSend) tea.Cmd {
	return func() tea.Msg {

		err := validateNonEmpty([][]string{
			{msg.uri, "missing channel URI"},
			{msg.body, "missing message body"},
		})
		if err != nil {
			return MessageError(err)
		}

		ent, err := m.keyring.EncodeEntry([]byte(msg.body), msg.uri)
		if err != nil {
			return MessageError(err)
		}

		req := &pb.FeedReq{
			ReqOp:    pb.ReqOp_AppendEntry,
			NewEntry: ent,
		}
		err = m.client.Send(req)
		if err != nil {
			return MessageError(err)
		}
		return MessageWorkDone{}
	}
}

// handlePeerAdd tells the vault to join one of its peers
func handlePeerAdd(m model, msg MessagePeerAdd) tea.Cmd {
	return func() tea.Msg {
		err := validateNonEmpty([][]string{
			{msg.id, "missing peer ID"},
			{msg.addr, "missing address"},
			{msg.pubkey, "missing public key"},
		})
		if err != nil {
			return MessageError(err)
		}

		peerUpdate := &pb.Peer{
			Op:  pb.PeerUpdateOp_Upsert,
			ID:  msg.id,
			Key: []byte(msg.pubkey),
			Multiaddrs: [][]byte{
				[]byte(msg.addr),
			},
		}
		body, err := proto.Marshal(peerUpdate)
		if err != nil {
			return MessageError(err)
		}

		ent, err := m.keyring.EncodeEntry([]byte(body), msg.uri)
		if err != nil {
			return MessageError(err)
		}

		req := &pb.FeedReq{
			ReqOp:    pb.ReqOp_AppendEntry,
			NewEntry: ent,
		}
		err = m.client.Send(req)
		if err != nil {
			return MessageError(err)
		}
		return MessageWorkDone{}
	}
}

// handleMemberAdd adds a new Member key to our keyring so we can decode
// its messages
func handleMemberAdd(m model, msg MessageMemberAdd) tea.Cmd {
	return func() tea.Msg {
		err := validateNonEmpty([][]string{
			{msg.nick, "missing nickname"},
			{msg.id, "missing member ID"},
			{msg.uri, "missing channel URI"},
			{msg.pubkey, "missing public key"},
		})
		if err != nil {
			return MessageError(err)
		}
		memberID := keyring.MemberIDFromString(msg.id)
		pubkey, err := keyring.MemberPublicKeyDecodeBase64(msg.pubkey)
		if err != nil {
			return MessageError(errors.New("public key was incorrectly formatted"))
		}
		m.keyring.AddMemberKey(msg.uri, memberID, pubkey)
		return MessageWorkDone{}
	}
}

func validateNonEmpty(tests [][]string) error {
	for _, test := range tests {
		if test[0] == "" {
			return errors.New(test[1])
		}
	}
	return nil
}
