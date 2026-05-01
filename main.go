package main

import (
	"github.com/koudis/bootstrap-ai-coding/internal/cmd"

	// Blank import wires the Claude Code agent into the registry via init().
	// Add future agents here — no other files change.
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/claude"
)

func main() {
	cmd.Execute()
}
