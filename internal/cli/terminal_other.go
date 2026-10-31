//go:build !linux && !darwin && !windows

package cli

import "os"

func terminalDevice(f *os.File) bool { return false }
