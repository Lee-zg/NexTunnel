//go:build !darwin

package macoshelper

import (
	"context"
	"fmt"
	"os"
)

func (c *Client) CreateTUN(ctx context.Context, req CreateTUNRequest) (*os.File, CreateTUNResult, error) {
	return nil, CreateTUNResult{}, fmt.Errorf("macOS helper TUN creation is unsupported on this platform")
}
