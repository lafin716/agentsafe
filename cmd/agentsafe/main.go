package main

import (
	"fmt"
	"os"

	"github.com/agentsafe/agentsafe/internal/app"
)

func main() {
	if err := app.NewRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
