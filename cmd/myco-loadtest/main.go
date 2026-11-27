// myco-loadtest drives a new, isolated local Hub, never an existing Hub.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/myco-ai/myco/internal/loadtest"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if e := loadtest.Main(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
