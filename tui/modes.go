package main

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/protobuf/proto"

	vclient "github.com/plan-systems/plan-vault-libp2p/client"
	"github.com/plan-systems/plan-vault-libp2p/keyring"
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
)

const (
	ModeSelector = iota
	ModeChannelOpen
	ModeChannelClose
	ModeSend
	ModePeerAdd
	ModeMemberAdd
	ModeConnector
)

func newChannelOpenMode(client *vclient.Client) inputMode {
	mode := newInputMode("Open Channel",
		[]inputModeOpt{
			{
				name:        "Channel URI",
				placeholder: "Enter channel URI to open",
			},
		})

	workFunc := func(mode inputMode) tea.Cmd {
		return func() tea.Msg {
			uri := mode.inputs[0].Value()
			err := validateNonEmpty([][]string{{uri, "missing channel URI"}})
			if err != nil {
				return MessageError(err)
			}
			err = client.OpenURI(uri)
			if err != nil {
				return MessageError(err)
			}
			mode.Reset()
			return switchMode(ModeSelector)
		}
	}

	mode.workFunc = workFunc
	return mode
}

func newChannelCloseMode(client *vclient.Client) inputMode {
	mode := newInputMode("Close Channel",
		[]inputModeOpt{
			{
				name:        "Channel URI",
				placeholder: "Enter channel URI to close",
			},
		})

	workFunc := func(mode inputMode) tea.Cmd {
		return func() tea.Msg {
			uri := mode.inputs[0].Value()
			err := validateNonEmpty([][]string{{uri, "missing channel URI"}})
			if err != nil {
				return MessageError(err)
			}
			err = client.CloseURI(uri)
			if err != nil {
				return MessageError(err)
			}
			mode.Reset()
			return switchMode(ModeSelector)
		}
	}

	mode.workFunc = workFunc
	return mode
}

func newSendMode(client *vclient.Client, kr *keyring.KeyRing) inputMode {
	mode := newInputMode("Send Messages",
		[]inputModeOpt{
			{
				name:        "Channel URI",
				placeholder: "Enter channel URI to send on",
			},
			{
				name:        "Message",
				placeholder: "Enter message",
			},
		})

	workFunc := func(mode inputMode) tea.Cmd {
		return func() tea.Msg {
			uri := mode.inputs[0].Value()
			body := mode.inputs[1].Value()

			err := validateNonEmpty([][]string{
				{uri, "missing channel URI"},
				{body, "missing message body"},
			})
			if err != nil {
				return MessageError(err)
			}

			ent, err := kr.EncodeEntry([]byte(body), uri)
			if err != nil {
				return MessageError(err)
			}

			req := &pb.FeedReq{
				ReqOp:    pb.ReqOp_AppendEntry,
				NewEntry: ent,
			}
			client.Send(req)

			mode.index = 1
			mode.inputs[1].Reset()
			return nil
		}
	}

	mode.workFunc = workFunc
	return mode
}

func newPeerAddMode(client *vclient.Client, kr *keyring.KeyRing, discoveryURI string) inputMode {
	mode := newInputMode("Add Peer Vault",
		[]inputModeOpt{
			{
				name:        "Peer ID",
				placeholder: "Enter peer member ID",
			},
			{
				name:        "Peer address",
				placeholder: "Enter peer multiaddr (ex. /ip4/192.168.1.1/tcp/5091)",
			},
			{
				name:        "Public Key",
				placeholder: "Enter base64-encoded public key",
			},
		})

	workFunc := func(mode inputMode) tea.Cmd {
		return func() tea.Msg {
			id := mode.inputs[0].Value()
			addr := mode.inputs[1].Value()
			pubkey := mode.inputs[2].Value()

			err := validateNonEmpty([][]string{
				{id, "missing peer ID"},
				{addr, "missing address"},
				{pubkey, "missing public key"},
			})
			if err != nil {
				return MessageError(err)
			}

			peerUpdate := &pb.Peer{
				Op:  pb.PeerUpdateOp_Upsert,
				ID:  id,
				Key: []byte(pubkey),
				Multiaddrs: [][]byte{
					[]byte(addr),
				},
			}
			body, err := proto.Marshal(peerUpdate)
			if err != nil {
				return MessageError(err)
			}

			ent, err := kr.EncodeEntry([]byte(body), discoveryURI)
			if err != nil {
				return MessageError(err)
			}

			req := &pb.FeedReq{
				ReqOp:    pb.ReqOp_AppendEntry,
				NewEntry: ent,
			}
			client.Send(req)

			mode.Reset()
			return switchMode(ModeSelector)
		}
	}

	mode.workFunc = workFunc
	return mode
}

func newMemberAddMode(kr *keyring.KeyRing) inputMode {
	mode := newInputMode("Add Member",
		[]inputModeOpt{
			{
				name:        "Nickname",
				placeholder: "Enter a name to display for this member",
			},
			{
				name:        "Member ID",
				placeholder: "Enter member ID",
			},
			{
				name:        "Channel URI",
				placeholder: "Enter channel URI where this member is active",
			},
			{
				name:        "Public Key",
				placeholder: "Enter base64-encoded public key",
			},
		})

	workFunc := func(mode inputMode) tea.Cmd {
		return func() tea.Msg {
			nick := mode.inputs[0].Value()
			id := mode.inputs[1].Value()
			uri := mode.inputs[2].Value()
			key := mode.inputs[3].Value()

			err := validateNonEmpty([][]string{
				{nick, "missing nickname"},
				{id, "missing member ID"},
				{uri, "missing channel URI"},
				{key, "missing public key"},
			})
			if err != nil {
				return MessageError(err)
			}

			memberID := keyring.MemberIDFromString(id)
			pubkey, err := keyring.MemberPublicKeyDecodeBase64(key)
			if err != nil {
				return MessageError(errors.New("public key was incorrectly formatted"))
			}

			kr.AddMemberKey(uri, memberID, pubkey)
			mode.Reset()
			return switchMode(ModeSelector)
		}
	}

	mode.workFunc = workFunc
	return mode
}

func validateNonEmpty(tests [][]string) error {
	for _, test := range tests {
		if test[0] == "" {
			return errors.New(test[1])
		}
	}
	return nil
}
