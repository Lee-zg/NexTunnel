//go:build darwin

package macoshelper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func (c *Client) CreateTUN(ctx context.Context, req CreateTUNRequest) (*os.File, CreateTUNResult, error) {
	if err := ValidateCreateTUNRequest(req); err != nil {
		return nil, CreateTUNResult{}, err
	}
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, CreateTUNResult{}, err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(request{
		Action:          actionCreateTUN,
		ProtocolVersion: ProtocolVersion,
		CreateTUN:       &req,
	}); err != nil {
		return nil, CreateTUNResult{}, err
	}

	payload := make([]byte, 4096)
	oob := make([]byte, unix.CmsgSpace(4))
	n, oobn, _, _, err := conn.ReadMsgUnix(payload, oob)
	if err != nil {
		return nil, CreateTUNResult{}, err
	}
	var resp response
	if err := json.Unmarshal(payload[:n], &resp); err != nil {
		return nil, CreateTUNResult{}, err
	}
	if resp.ProtocolVersion != "" && resp.ProtocolVersion != ProtocolVersion {
		return nil, CreateTUNResult{}, fmt.Errorf("helper protocol mismatch: got %s want %s", resp.ProtocolVersion, ProtocolVersion)
	}
	if !resp.OK {
		return nil, CreateTUNResult{}, errors.New(resp.Error)
	}
	if resp.CreateTUN == nil {
		return nil, CreateTUNResult{}, fmt.Errorf("helper returned no TUN metadata")
	}
	cmsgs, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, CreateTUNResult{}, err
	}
	for _, cmsg := range cmsgs {
		fds, err := unix.ParseUnixRights(&cmsg)
		if err != nil {
			continue
		}
		if len(fds) > 0 {
			file := os.NewFile(uintptr(fds[0]), "/dev/"+resp.CreateTUN.Interface)
			return file, *resp.CreateTUN, nil
		}
	}
	return nil, CreateTUNResult{}, fmt.Errorf("helper did not pass a TUN file descriptor")
}
