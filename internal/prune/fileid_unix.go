//go:build unix

package prune

import (
	"io/fs"
	"syscall"
)

// identify returns the identity of the file described by info. The
// comma-ok form lets callers fall back to pairwise os.SameFile
// comparison when the platform does not expose the underlying stat.
func identify(info fs.FileInfo) (fileIdentity, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileIdentity{}, false
	}
	return fileIdentity{
		device: uint64(stat.Dev),
		inode:  uint64(stat.Ino),
		links:  uint64(stat.Nlink),
	}, true
}
