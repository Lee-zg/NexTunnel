//go:build !windows

package macoshelper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/nextunnel/desktop/internal/virtualnet"
)

func (c *Client) Status(ctx context.Context) (Status, error) {
	resp, err := c.roundTrip(ctx, request{Action: actionStatus, ProtocolVersion: ProtocolVersion})
	if err != nil {
		return Status{Running: false, SocketPath: c.normalizedSocketPath(), Message: err.Error()}, err
	}
	return Status{
		Running:         resp.OK,
		Version:         resp.Version,
		ProtocolVersion: resp.ProtocolVersion,
		Signed:          resp.Signed,
		SocketPath:      c.normalizedSocketPath(),
		Message:         resp.Message,
	}, nil
}

func (c *Client) ApplyVirtualNetwork(cfg virtualnet.Config) (virtualnet.State, error) {
	if err := ValidateVirtualNetworkConfig(cfg); err != nil {
		return virtualnet.State{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()
	resp, err := c.roundTrip(ctx, request{
		Action:          actionApplyRoute,
		ProtocolVersion: ProtocolVersion,
		VirtualNetwork:  &cfg,
	})
	if err != nil {
		return virtualnet.State{}, err
	}
	if !resp.OK {
		return virtualnet.State{}, errors.New(resp.Error)
	}
	if resp.State == nil {
		return virtualnet.State{}, fmt.Errorf("helper returned no virtual network state")
	}
	return *resp.State, nil
}

func (c *Client) ResetVirtualNetwork(state virtualnet.State) (virtualnet.State, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout())
	defer cancel()
	resp, err := c.roundTrip(ctx, request{
		Action:          actionResetRoute,
		ProtocolVersion: ProtocolVersion,
		State:           &state,
	})
	if err != nil {
		return state, err
	}
	if !resp.OK {
		return state, errors.New(resp.Error)
	}
	if resp.State == nil {
		state.Applied = false
		return state, nil
	}
	return *resp.State, nil
}

func (c *Client) roundTrip(ctx context.Context, req request) (*response, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}
	var resp response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}
	if resp.ProtocolVersion != "" && resp.ProtocolVersion != ProtocolVersion {
		return nil, fmt.Errorf("helper protocol mismatch: got %s want %s", resp.ProtocolVersion, ProtocolVersion)
	}
	return &resp, nil
}

func (c *Client) dial(ctx context.Context) (*net.UnixConn, error) {
	timeout := c.timeout()
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "unix", c.normalizedSocketPath())
	if err != nil {
		return nil, err
	}
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("helper connection is %T, want *net.UnixConn", conn)
	}
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = unixConn.SetDeadline(deadline)
	return unixConn, nil
}
