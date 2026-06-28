package utils

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// DownloadCompleteMsg is sent after a version is downloaded and
// successfully extracted.
type DownloadCompleteMsg struct {
	Version string
	Path    string
}

func DownloadAndInstall(version GoVersion) tea.Cmd {
	return func() tea.Msg {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ErrMsg(err)
		}
		goVersionsDir := filepath.Join(homeDir, ".govm", "versions")
		downloadDir := filepath.Join(homeDir, ".govm", "downloads")
		for _, dir := range []string{goVersionsDir, downloadDir} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return ErrMsg(err)
			}
		}
		versionDir := filepath.Join(goVersionsDir, fmt.Sprintf("go%s", version.Version))
		if _, err := os.Stat(versionDir); err == nil {
			if err := os.RemoveAll(versionDir); err != nil {
				return ErrMsg(fmt.Errorf("failed to remove existing installation: %v", err))
			}
		}
		downloadPath := filepath.Join(downloadDir, version.Filename)
		if _, err := os.Stat(downloadPath); err == nil {
			if err := os.Remove(downloadPath); err != nil {
				return ErrMsg(fmt.Errorf("failed to remove existing download: %v", err))
			}
		}
		resp, err := http.Get(version.URL)
		if err != nil {
			return ErrMsg(err)
		}
		defer resp.Body.Close()
		out, err := os.Create(downloadPath)
		if err != nil {
			return ErrMsg(err)
		}
		defer out.Close()
		written, err := io.Copy(out, resp.Body)
		if err != nil {
			return ErrMsg(err)
		}
		if written == 0 {
			return ErrMsg(fmt.Errorf("downloaded empty file"))
		}
		out.Close()
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			if strings.HasSuffix(version.Filename, ".zip") {
				cmd = exec.Command("powershell", "-Command",
					fmt.Sprintf("Expand-Archive -Path \"%s\" -DestinationPath \"%s\" -Force",
						downloadPath, goVersionsDir))
			} else {
				return ErrMsg(fmt.Errorf("unsupported archive format for Windows: %s", version.Filename))
			}
		} else {
			if strings.HasSuffix(version.Filename, ".tar.gz") {
				cmd = exec.Command("tar", "-xzf", downloadPath, "-C", goVersionsDir)
			} else {
				return ErrMsg(fmt.Errorf("unsupported archive format for Unix: %s", version.Filename))
			}
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			return ErrMsg(fmt.Errorf("extraction error: %v\nOutput: %s", err, string(output)))
		}
		if runtime.GOOS != "windows" {
			goBin := filepath.Join(versionDir, "bin", "go")
			if _, err := os.Stat(goBin); err == nil {
				os.Chmod(goBin, 0755)
			}
		}
		goBin := filepath.Join(versionDir, "bin", "go")
		if _, err := os.Stat(goBin); os.IsNotExist(err) {
			entries, _ := os.ReadDir(goVersionsDir)
			for _, entry := range entries {
				if entry.IsDir() && strings.HasPrefix(entry.Name(), "go") {
					testPath := filepath.Join(goVersionsDir, entry.Name(), "bin", "go")
					if _, err := os.Stat(testPath); err == nil {
						sourcePath := filepath.Join(goVersionsDir, entry.Name())
						if sourcePath != versionDir {
							if err := os.Rename(sourcePath, versionDir); err != nil {
								return ErrMsg(fmt.Errorf("failed to rename directory: %v", err))
							}
						}
						break
					}
				}
			}
		}
		if _, err := os.Stat(goBin); os.IsNotExist(err) {
			return ErrMsg(fmt.Errorf("installation failed: Go binary not found at %s", goBin))
		}
		verifyCmd := exec.Command(goBin, "version")
		verifyOutput, err := verifyCmd.CombinedOutput()
		if err != nil {
			return ErrMsg(fmt.Errorf("Go binary verification failed: %v\nOutput: %s", err, string(verifyOutput)))
		}
		// Remove the existing downloads since they should be installed
		if err := os.Remove(downloadPath); err != nil {
			// Just log the error but don't fail the installation
			fmt.Printf("Warning: failed to clean up download file: %v\n", err)
		}
		return DownloadCompleteMsg{Version: version.Version, Path: versionDir}
	}
}
