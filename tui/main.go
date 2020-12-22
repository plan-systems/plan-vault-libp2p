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
	pb "github.com/plan-systems/plan-vault-libp2p/protos"
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

type MessageContent = string
type MessageSwitchMode = int
type MessageError = error
type MessageEntry = *pb.Msg
type MessageWorkDone = struct{}

func initialModel(cfg *config.Config) tea.Model {
	ctx, cancel := context.WithCancel(context.Background())

	// if either of these fail, we'll just bail and not bother
	// setting up the UI
	client := connect(ctx, cfg.ServerConfig)
	kr := openKeyring(cfg.P2PConfig)

	m := model{
		ctx:     ctx,
		cancel:  cancel,
		client:  client,
		keyring: kr,
		modes: []tea.Model{
			newSelectorMode(),
			newChannelOpenMode(client),
			newChannelCloseMode(client),
			newSendMode(client, kr),
			newPeerAddMode(client, kr, cfg.P2PConfig.URI),
			newMemberAddMode(kr),
		}}
	m.mode = ModeSelector

	go pollClient(m)

	return m
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

	client  *vclient.Client
	keyring *keyring.KeyRing
	entryCh chan MessageEntry

	content  string
	ready    bool
	viewport viewport.Model

	modes []tea.Model
	mode  int // index into modes
}

func (m model) Init() tea.Cmd {
	return nil
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

	case MessageSwitchMode:
		m.mode = msg

	case MessageEntry:
		cmds = append(cmds, handleEntry(m.keyring, msg))
		cmds = append(cmds, pollNextEntry(m))

	case MessageContent:
		m.content += msg
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

func switchMode(mode int) tea.Cmd {
	return func() tea.Msg {
		return MessageSwitchMode(mode)
	}
}
