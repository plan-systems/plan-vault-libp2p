package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
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
