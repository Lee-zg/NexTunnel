//go:build !darwin

package macoshelper

import (
	"context"
	"fmt"
)

type ServerOptions struct {
	SocketPath string
	Version    string
	Signed     bool
}

func RunServer(ctx context.Context, opts ServerOptions) error {
	return fmt.Errorf("macOS helper server is only available on darwin")
}
