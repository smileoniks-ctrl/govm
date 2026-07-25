//go:build windows

package state

import "golang.org/x/sys/windows"

func replaceMarkerFile(source, target string) error {
	sourceUTF16, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	targetUTF16, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(
		sourceUTF16,
		targetUTF16,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}
