package command

import "testing"

func TestRootCommandRegistersHelperCommands(t *testing.T) {
	root := NewRootCommand("test")
	helper, _, err := root.Find([]string{"helper"})
	if err != nil {
		t.Fatalf("find helper command: %v", err)
	}
	if helper == nil || helper.Name() != "helper" {
		t.Fatalf("helper command not registered")
	}
	for _, name := range []string{"status", "install", "restart", "uninstall"} {
		child, _, err := helper.Find([]string{name})
		if err != nil {
			t.Fatalf("find helper %s: %v", name, err)
		}
		if child == nil || child.Name() != name {
			t.Fatalf("helper %s command not registered", name)
		}
	}
}
