//go:build darwin

package macoshelper

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestAuthorizedPeerAllowsRootOrAdminGroup(t *testing.T) {
	adminGID := uint32(80)
	if !authorizedPeer(&unix.Xucred{Uid: 0}, adminGID) {
		t.Fatal("root peer should be authorized")
	}
	if !authorizedPeer(&unix.Xucred{Uid: 501, Ngroups: 1, Groups: [16]uint32{adminGID}}, adminGID) {
		t.Fatal("admin group peer should be authorized")
	}
	if authorizedPeer(&unix.Xucred{Uid: 501, Ngroups: 1, Groups: [16]uint32{20}}, adminGID) {
		t.Fatal("non-admin peer must not be authorized")
	}
}
