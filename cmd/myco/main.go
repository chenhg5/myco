// Command myco is the single binary for the hub, the CLI and the runner.
package main

import (
	"os"

	"github.com/chenhg5/myco/internal/cli"
)

func main() { os.Exit(cli.Main(os.Args[1:])) }
