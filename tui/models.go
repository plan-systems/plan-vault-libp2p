package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	te "github.com/muesli/termenv"
)

const (
	blurredPrompt    = "> "
	textColorFocused = "205"
	textColorBlurred = "240"
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

var (
	color               = te.ColorProfile().Color
	focusedPrompt       = te.String("> ").Foreground(color("205")).String()
	focusedSubmitButton = "[ " + te.String("Submit").Foreground(color("205")).String() + " ]"
	blurredSubmitButton = "[ " + te.String("Submit").Foreground(color("240")).String() + " ]"
	errorButton         = "[ " + te.String("ERROR").Foreground(color("100")).String() + " ]"
)

type choice struct {
	text string
	id   int
}

type selectorMode struct {
	choices []choice
	cursor  int
}

func newSelectorMode() selectorMode {
	return selectorMode{
		choices: []choice{
			{"open channel", ModeChannelOpen},
			{"close channel", ModeChannelClose},
			{"send messages", ModeSend},
			{"add remote peer", ModePeerAdd},
			{"add member", ModeMemberAdd},
		},
	}
}

func (m selectorMode) Init() tea.Cmd {
	return nil
}

func (m selectorMode) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = len(m.choices) - 1
			}
		case "down":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			} else {
				m.cursor = 0
			}
		case "enter", " ":
			return m, switchMode(m.choices[m.cursor].id)
		case "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m selectorMode) View() string {
	content := ""
	for i, choice := range m.choices {

		cursor := "[ ]"
		if m.cursor == i {
			cursor = "[x]"
		}
		content += fmt.Sprintf("%s %s\n", cursor, choice.text)
	}

	// The footer
	content += "\nSelect an option and press Enter."
	content += "\nPress Ctrl+C at any time to quit.\n"
	return content
}

func newChannelOpenMode() inputMode { // }client *vclient.Client) inputMode {
	return newInputMode("Open Channel",
		[]inputModeOpt{
			{
				name:        "Channel URI",
				placeholder: "Enter channel URI to open",
			},
		},
		func(mode inputMode) tea.Cmd {
			return func() tea.Msg {
				return MessageOpenURI{uri: mode.inputs[0].Value()}
			}
		})
}

func newChannelCloseMode() inputMode {
	return newInputMode("Close Channel",
		[]inputModeOpt{
			{
				name:        "Channel URI",
				placeholder: "Enter channel URI to close",
			},
		},
		func(mode inputMode) tea.Cmd {
			return func() tea.Msg {
				return MessageCloseURI{uri: mode.inputs[0].Value()}
			}
		})
}

func newSendMode() inputMode {
	return newInputMode("Send Messages",
		[]inputModeOpt{
			{
				name:        "Channel URI",
				placeholder: "Enter channel URI to send on",
			},
			{
				name:        "Message",
				placeholder: "Enter message",
			},
		},
		func(mode inputMode) tea.Cmd {
			return func() tea.Msg {
				uri := mode.inputs[0].Value()
				body := mode.inputs[1].Value()
				mode.index = 1
				mode.inputs[1].Reset()
				return MessageSend{uri: uri, body: body}
			}
		})
}

func newPeerAddMode(discoveryURI string) inputMode {
	return newInputMode("Add Peer Vault",
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
		},
		func(mode inputMode) tea.Cmd {
			return func() tea.Msg {
				return MessagePeerAdd{
					uri:    discoveryURI,
					id:     mode.inputs[0].Value(),
					addr:   mode.inputs[1].Value(),
					pubkey: mode.inputs[2].Value(),
				}
			}
		})
}

func newMemberAddMode() inputMode {
	return newInputMode("Add Member",
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
		},
		func(mode inputMode) tea.Cmd {
			return func() tea.Msg {
				return MessageMemberAdd{
					nick:   mode.inputs[0].Value(),
					id:     mode.inputs[1].Value(),
					uri:    mode.inputs[2].Value(),
					pubkey: mode.inputs[3].Value(),
				}
			}
		},
	)
}
