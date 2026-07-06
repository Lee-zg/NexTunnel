//go:build !darwin

package main

func runMacOSHelperAdminCommandIfRequested([]string) bool {
	return false
}
