package main

import (
	"os"

	"github.com/felixgeelhaar/roady/internal/infrastructure/cli"
)

// exit is os.Exit, swappable so a test can observe the failure path.
var exit = os.Exit

func main() {
	if err := cli.Execute(); err != nil {
		exit(1)
	}
}
