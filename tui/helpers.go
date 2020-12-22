package main

import (
	te "github.com/muesli/termenv"
)

const (
	blurredPrompt    = "> "
	textColorFocused = "205"
	textColorBlurred = "240"
)

var (
	color               = te.ColorProfile().Color
	focusedPrompt       = te.String("> ").Foreground(color("205")).String()
	focusedSubmitButton = "[ " + te.String("Submit").Foreground(color("205")).String() + " ]"
	blurredSubmitButton = "[ " + te.String("Submit").Foreground(color("240")).String() + " ]"
	errorButton         = "[ " + te.String("ERROR").Foreground(color("100")).String() + " ]"
)
