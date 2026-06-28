package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	tea "charm.land/bubbletea/v2"
)

// SwitchCompletedMsg reports the result of a successful switch.
// ShimInPath is true when the govm shim directory is already on the
// user's PATH, so the caller can decide whether to remind the user
// to add it.
type SwitchCompletedMsg struct {
	Version    string
	ShimInPath bool
}

func SwitchVersion(version GoVersion) tea.Cmd {
	return func() tea.Msg {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ErrMsg(err)
		}
		if err := SetupShimDirectory(); err != nil {
			return ErrMsg(err)
		}
		shimDir := filepath.Join(homeDir, ".govm", "shim")
		versionBinDir := filepath.Join(version.Path, "bin")
		if _, err := os.Stat(versionBinDir); os.IsNotExist(err) {
			return ErrMsg(fmt.Errorf("go version directory not found: %s", versionBinDir))
		}
		entries, err := os.ReadDir(versionBinDir)
		if err != nil {
			return ErrMsg(fmt.Errorf("failed to read bin directory: %v", err))
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				binName := entry.Name()
				targetBin := filepath.Join(versionBinDir, binName)
				shimPath := filepath.Join(shimDir, binName)
				os.Remove(shimPath)
				if runtime.GOOS == "windows" {
					shimContent := fmt.Sprintf(`@echo off
"%s" %%*
`, targetBin)
					if err := os.WriteFile(shimPath+".bat", []byte(shimContent), 0755); err != nil {
						return ErrMsg(fmt.Errorf("failed to create shim for %s: %v", binName, err))
					}
				} else {
					shimContent := fmt.Sprintf(`#!/usr/bin/env bash
"%s" "$@"
`, targetBin)
					if err := os.WriteFile(shimPath, []byte(shimContent), 0755); err != nil {
						return ErrMsg(fmt.Errorf("failed to create shim for %s: %v", binName, err))
					}
					if err := os.Chmod(shimPath, 0755); err != nil {
						return ErrMsg(fmt.Errorf("failed to make shim executable: %v", err))
					}
				}
			}
		}
		versionFile := filepath.Join(homeDir, ".govm", "active_version")
		if err := os.WriteFile(versionFile, []byte(version.Version), 0644); err != nil {
			return ErrMsg(fmt.Errorf("failed to update active version file: %v", err))
		}
		shimInPath := IsShimInPath()
		return SwitchCompletedMsg{
			Version:    version.Version,
			ShimInPath: shimInPath,
		}
	}
}
