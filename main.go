package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ja-carroll/kube-tui/internal/ui"
)

func main() {
	// Phase 1: Landing page — context selection and connection.
	// This runs as its own Bubble Tea program. When the user selects a
	// context and connects successfully, it exits and passes the client
	// back via the model's Client field.
	landing := ui.NewLanding()
	p := tea.NewProgram(landing, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Extract the connected client from the landing model.
	landingModel, ok := result.(ui.LandingModel)
	if !ok || landingModel.Client == nil {
		// User quit without connecting
		os.Exit(0)
	}

	// Phase 2: Main application — the full TUI.
	// A second Bubble Tea program gets its own alt screen, so the
	// transition from landing → main is seamless.
	model := ui.New(landingModel.Client)
	p = tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
