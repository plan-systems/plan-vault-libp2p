package main

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
)

type inputMode struct {
	title       string
	inputs      []textinput.Model
	inputNames  []string
	namePadding int // pre-calculated for View

	spinner      spinner.Model
	submitButton string

	submitFunc func(inputMode) tea.Cmd // should close over the inputMode to validate

	working bool
	index   int
	err     error
}

type inputModeOpt struct {
	name        string
	placeholder string
}

func newInputMode(title string, opts []inputModeOpt, submitFunc func(inputMode) tea.Cmd) inputMode {

	m := inputMode{
		title:      title,
		inputs:     []textinput.Model{},
		inputNames: []string{},
	}

	for _, opt := range opts {
		in := textinput.NewModel()
		in.Placeholder = opt.placeholder
		in.Prompt = blurredPrompt
		in.TextColor = textColorFocused
		in.CharLimit = 60 // TODO: figure this out
		in.Width = 60

		m.inputs = append(m.inputs, in)
		m.inputNames = append(m.inputNames, opt.name)
	}

	for _, name := range m.inputNames {
		if len(name) > m.namePadding {
			m.namePadding = len(name)
		}
	}

	m.inputs[0].Focus()
	m.inputs[0].Prompt = focusedPrompt

	s := spinner.NewModel()
	s.Spinner = spinner.Dot
	m.spinner = s

	m.submitButton = blurredSubmitButton
	m.submitFunc = submitFunc

	return m
}

func (m inputMode) Init() tea.Cmd {
	return textinput.Blink
}

func (m inputMode) Reset() {
	m.err = nil
	m.index = 0
	m.working = false
	m.submitButton = blurredSubmitButton

	for _, input := range m.inputs {
		input.Reset()
	}
	m.inputs[0].Focus()
	m.inputs[0].Prompt = focusedPrompt
}

func (m inputMode) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case MessageError:
		m.working = false
		m.err = msg
		return m, nil
	case MessageWorkDone:
		// TODO: this is janky b/c it assumes we only have one bit of
		// work going at once. Make it so we detect what mode this was
		// for and drop it otherwise (yet another reason why having a
		// two-way stream for commands is so meh)
		m.Reset()
		return m, switchMode(ModeSelector)
	case tea.KeyMsg:
		m.err = nil

		s := msg.String()
		switch s {
		case "esc":
			m.Reset()
			return m, switchMode(ModeSelector)
		case "tab", "shift+tab", "enter", "up", "down":
			if s == "enter" && m.index == len(m.inputs) {
				m.working = true
				return m, tea.Batch(spinner.Tick, m.submitFunc(m))
			}
			if s == "up" || s == "shift-tab" {
				m.index--
			} else {
				m.index++
			}
			if m.index > len(m.inputs) {
				m.index = 0
			} else if m.index < 0 {
				m.index = len(m.inputs)
			}
			for i := 0; i <= len(m.inputs)-1; i++ {
				if i == m.index {
					// Set focused state
					m.inputs[i].Focus()
					m.inputs[i].Prompt = focusedPrompt
					m.inputs[i].TextColor = textColorFocused
					continue
				}
				// Remove focused state
				m.inputs[i].Blur()
				m.inputs[i].Prompt = blurredPrompt
				m.inputs[i].TextColor = ""
			}
			if m.index == len(m.inputs) {
				m.submitButton = focusedSubmitButton
			} else {
				m.submitButton = blurredSubmitButton
			}
			return m, nil

		}
	}
	return updateInputs(msg, m)
}

// Pass messages and models through to text input components. Only text inputs
// with Focus() set will respond, so it's safe to simply update all of them
// here without any further logic.
func updateInputs(msg tea.Msg, m inputMode) (inputMode, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	for i, input := range m.inputs {
		m.inputs[i], cmd = input.Update(msg)
		cmds = append(cmds, cmd)
	}

	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m inputMode) View() string {
	content := fmt.Sprintf(" %s:\n\n", m.title)

	inputFmt := " %" + strconv.Itoa(m.namePadding) + "s  %s\n"

	for i, input := range m.inputs {
		content += fmt.Sprintf(inputFmt, m.inputNames[i], input.View())
	}
	content += "\n"

	if m.working {
		content += " "
		content += termenv.String(m.spinner.View()).Foreground(color("205")).String()
		content += " working...\n"
	} else if m.err != nil {
		// TODO: how do we clean up from wrapped-around error messages?
		content += " " + errorButton + " " + m.err.Error() + "\n"
	} else {
		content += " " + m.submitButton
	}
	return content
}
