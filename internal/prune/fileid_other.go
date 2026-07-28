//go:build !unix

package prune

import "io/fs"

// identify always reports false here: this build has no portable way to
// read a file's identity, so hard-link detection stays with os.SameFile.
func identify(fs.FileInfo) (fileIdentity, bool) {
	return fileIdentity{}, false
}
