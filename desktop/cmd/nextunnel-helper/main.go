package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nextunnel/desktop/internal/macoshelper"
)

var (
	version = "0.6.4-alpha"
	signed  = "false"
)

func main() {
	var socketPath string
	flag.StringVar(&socketPath, "socket", macoshelper.DefaultSocketPath, "Unix socket path")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err := macoshelper.RunServer(ctx, macoshelper.ServerOptions{
		SocketPath: socketPath,
		Version:    version,
		Signed:     signed == "true",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
