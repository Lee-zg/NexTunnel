//go:build darwin || linux || freebsd || openbsd || netbsd

package macoshelperctl

import "os"

func defaultEUID() int {
	return os.Geteuid()
}
