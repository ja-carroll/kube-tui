package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ja-carroll/kube-tui/internal/k8s"
	"github.com/ja-carroll/kube-tui/internal/ui"
)

func main() {
	// Step 1: Connect to Kubernetes
	client, err := k8s.NewClient()
	if err != nil {
		fmt.Printf("Failed to connect to Kubernetes: %v\n", err)
		os.Exit(1)
	}

	// Step 2: Create the UI model with the client
	model := ui.New(client)

	// Step 3: Run the Bubble Tea program
	// tea.WithAltScreen() switches the terminal to an "alternate screen buffer".
	// This is the same thing vim, htop, less, etc. use — your app gets its own
	// full-screen canvas, and when it exits, the original terminal content is
	// restored. Without this, the UI renders inline and scrolls off.
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
