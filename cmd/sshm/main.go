package main

import (
	"os"

	"github.com/Sebasouthwell/sshm/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
