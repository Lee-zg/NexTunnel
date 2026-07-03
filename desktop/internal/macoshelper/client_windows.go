//go:build windows

package macoshelper

import (
	"context"
	"fmt"

	"github.com/nextunnel/desktop/internal/virtualnet"
)

func (c *Client) Status(ctx context.Context) (Status, error) {
	err := fmt.Errorf("macOS helper is unsupported on windows")
	return Status{Running: false, SocketPath: c.normalizedSocketPath(), Message: err.Error()}, err
}

func (c *Client) ApplyVirtualNetwork(cfg virtualnet.Config) (virtualnet.State, error) {
	return virtualnet.State{}, fmt.Errorf("macOS helper is unsupported on windows")
}

func (c *Client) ResetVirtualNetwork(state virtualnet.State) (virtualnet.State, error) {
	return state, fmt.Errorf("macOS helper is unsupported on windows")
}
