package main

import (
	"github.com/koudis/bootstrap-ai-coding/internal/cmd"

	// Blank imports wire agent modules into the registry via their init() functions.
	// Add future agents here — no other files change.
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/augment"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/buildresources"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/claude"
)

func main() {
	cmd.Execute()
}
