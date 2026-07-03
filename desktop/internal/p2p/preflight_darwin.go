//go:build darwin

package p2p

import (
	"encoding/json"
	"net"
	"os"
	"time"
)

const (
	macOSHelperSocketPath      = "/var/run/nextunnel/helper.sock"
	macOSHelperProtocolVersion = "1"
)

func isProcessPrivileged() bool {
	return os.Geteuid() == 0
}

func isPEDLLArchitectureCompatible(string) (bool, string) {
	return true, "not required on this platform"
}

func detectMacOSHelperPreflight() macOSHelperPreflightResult {
	result := macOSHelperPreflightResult{required: true}
	if _, err := os.Stat(macOSHelperSocketPath); err != nil {
		result.detail = err.Error()
		return result
	}
	result.found = true
	conn, err := net.DialTimeout("unix", macOSHelperSocketPath, 800*time.Millisecond)
	if err != nil {
		result.detail = err.Error()
		return result
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(800 * time.Millisecond))
	request := map[string]string{
		"action":           "status",
		"protocol_version": macOSHelperProtocolVersion,
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		result.detail = err.Error()
		return result
	}
	var response struct {
		OK              bool   `json:"ok"`
		ProtocolVersion string `json:"protocol_version"`
		Version         string `json:"version"`
		Error           string `json:"error"`
		Message         string `json:"message"`
	}
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		result.detail = err.Error()
		return result
	}
	result.reachable = true
	result.version = response.Version
	if response.OK && response.ProtocolVersion == macOSHelperProtocolVersion {
		result.ready = true
		result.detail = response.Message
		return result
	}
	if response.Error != "" {
		result.detail = response.Error
	} else {
		result.detail = "helper protocol mismatch"
	}
	return result
}
