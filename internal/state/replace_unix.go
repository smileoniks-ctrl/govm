//go:build !windows

package state

import "os"

func replaceMarkerFile(source, target string) error {
	return os.Rename(source, target)
}
