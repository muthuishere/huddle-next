// Command huddle is the AI-native SDLC orchestrator CLI.
package main

import (
	"fmt"
	"os"

	"github.com/muthuishere/huddle/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "huddle:", err)
		os.Exit(1)
	}
}
