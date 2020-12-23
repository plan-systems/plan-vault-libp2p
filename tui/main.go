package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	vclient "github.com/plan-systems/plan-vault-libp2p/client"
	"github.com/plan-systems/plan-vault-libp2p/config"
	"github.com/plan-systems/plan-vault-libp2p/keyring"
	"github.com/plan-systems/plan-vault-libp2p/p2p"
)

func main() {
	// we re-use the config file from vault for convenience
	cfg := config.NewConfig()
	p := tea.NewProgram(initialModel(cfg))

	p.EnterAltScreen() // full screen
	defer p.ExitAltScreen()

	p.EnableMouseCellMotion() // mouse support
	defer p.DisableMouseCellMotion()

	if err := p.Start(); err != nil {
		fmt.Printf("could not start client: %s\n", err)
		os.Exit(1)
	}
}

func initialModel(cfg *config.Config) tea.Model {
	ctx, cancel := context.WithCancel(context.Background())

	// if this fails, we'll just bail and not bother setting up the UI
	kr := openKeyring(cfg.P2PConfig)

	return model{
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		keyring: kr,
		modes: []tea.Model{
			newSelectorMode(),
			newChannelOpenMode(),
			newChannelCloseMode(),
			newSendMode(),
			newPeerAddMode(cfg.P2PConfig.URI),
			newMemberAddMode(),
		},
	}
}

func openKeyring(cfg *p2p.Config) *keyring.KeyRing {
	kr, err := keyring.NewFromFile(cfg.URI, cfg.KeyFile)
	if err != nil {
		log.Fatalf("could not open keyring: %v", err)
	}
	return kr
}

type model struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    *config.Config

	client  *vclient.Client
	keyring *keyring.KeyRing

	content  string
	ready    bool
	viewport viewport.Model

	modes []tea.Model
	mode  int // index into modes
}

func (m model) Init() tea.Cmd {
	return connect(m)
}

const (
	headerHeight = 1
	footerHeight = 8
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancel()
			return m, tea.Quit
		default:
			// fallthru; we'll pass this message to the mode
		}

	case tea.WindowSizeMsg:
		verticalMargins := headerHeight + footerHeight

		if !m.ready {
			// wait until we have the window dimensions before
			// initializing the viewport
			m.viewport = viewport.Model{
				Width:  msg.Width,
				Height: msg.Height - verticalMargins}
			m.viewport.YPosition = headerHeight
			m.viewport.SetContent(m.content)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMargins
		}

	case MessageConnected:
		m.client = msg
		m.content += "connected!\n"
		cmds = append(cmds, pollNextEntry(m))

	case MessageSwitchMode:
		m.mode = msg

	case MessageEntry:
		cmds = append(cmds, handleEntry(m.keyring, msg))
		cmds = append(cmds, pollNextEntry(m))

	case MessageContent:
		m.content += msg

	case MessageOpenURI:
		cmds = append(cmds, handleOpenURI(m, msg))

	case MessageCloseURI:
		cmds = append(cmds, handleCloseURI(m, msg))

	case MessageSend:
		cmds = append(cmds, handleSendMessage(m, msg))

	case MessagePeerAdd:
		cmds = append(cmds, handlePeerAdd(m, msg))

	case MessageMemberAdd:
		cmds = append(cmds, handleMemberAdd(m, msg))
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	mode, cmd := m.modes[m.mode].Update(msg)
	m.modes[m.mode] = mode
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "\n  Connecting..." // not really
	}
	divider := strings.Repeat("─", m.viewport.Width)
	return m.viewport.View() + divider + m.modes[m.mode].View()
}
