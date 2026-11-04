package main

import (
	"os"

	"github.com/brotherlogic/newci/pkg/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
