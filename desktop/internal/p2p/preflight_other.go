//go:build !darwin

package p2p

func detectMacOSHelperPreflight() macOSHelperPreflightResult {
	return macOSHelperPreflightResult{}
}
