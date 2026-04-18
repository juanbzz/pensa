package main

import (
	"fmt"
	"os"

	"pensa.sh/pensa/internal/cli"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	return cli.Execute()
}
