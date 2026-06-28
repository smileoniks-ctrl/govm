package cli

import (
	"fmt"
	"github.com/smileoniks-ctrl/govm/internal/utils"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func InstallVersion(version string) {
	fmt.Printf("🔍 Looking for Go version matching %s...\n", version)
	matchedVersion, err := findMatchingVersion(version)
	if err != nil {
		fmt.Printf("❌ %s\n", err)
		return
	}
	fmt.Printf("📥 Installing Go %s...\n", matchedVersion.Version)
	done := make(chan bool)
	errCh := make(chan error)
	go func() {
		msg := utils.DownloadAndInstall(matchedVersion)()
		switch msg := msg.(type) {
		case utils.ErrMsg:
			errCh <- msg
		case utils.DownloadCompleteMsg:
			done <- true
		}
	}()
	spinChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinIdx := 0
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			fmt.Printf("\r✅ Successfully installed Go %s\n", matchedVersion.Version)
			fmt.Printf("👉 To activate this version, run: govm use %s\n", matchedVersion.Version)
			return
		case err := <-errCh:
			fmt.Printf("\r❌ Installation failed: %v\n", err)
			return
		case <-ticker.C:
			fmt.Printf("\r%s Installing Go %s...", spinChars[spinIdx], matchedVersion.Version)
			spinIdx = (spinIdx + 1) % len(spinChars)
		}
	}
}
func UseVersion(version string) {
	fmt.Printf("🔍 Looking for installed Go version matching %s...\n", version)
	matchedVersion, err := findInstalledVersion(version)
	if err != nil {
		fmt.Printf("❌ %s\n", err)
		return
	}
	fmt.Printf("🔄 Switching to Go %s...\n", matchedVersion.Version)
	msg := utils.SwitchVersion(matchedVersion)()
	switch msg := msg.(type) {
	case utils.ErrMsg:
		fmt.Printf("❌ Failed to switch version: %v\n", msg)
	case utils.SwitchCompletedMsg:
		fmt.Printf("✅ Switched to Go %s\n", matchedVersion.Version)
		if !utils.IsShimInPath() {
			fmt.Println("\n⚠️  GoVM is not in your PATH")
			fmt.Println(utils.GetShimPathInstructions())
		} else {
			fmt.Println("🚀 Run 'go version' in a new terminal to verify")
		}
	}
}
func ListVersions() {
	fmt.Println("📋 Installed Go Versions:")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("❌ Error getting home directory: %v\n", err)
		return
	}
	activeVersion := ""
	activeVersionFile := filepath.Join(homeDir, ".govm", "active_version")
	if versionBytes, err := os.ReadFile(activeVersionFile); err == nil {
		activeVersion = string(versionBytes)
	}
	goVersionsDir := filepath.Join(homeDir, ".govm", "versions")
	if _, err := os.Stat(goVersionsDir); os.IsNotExist(err) {
		fmt.Println("  No versions installed yet")
		return
	}
	entries, err := os.ReadDir(goVersionsDir)
	if err != nil {
		fmt.Printf("❌ Error reading versions directory: %v\n", err)
		return
	}
	if len(entries) == 0 {
		fmt.Println("  No versions installed yet")
		return
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "go") {
			version := strings.TrimPrefix(entry.Name(), "go")
			if version == activeVersion {
				fmt.Printf("  %s %s\n", version, "✓ (active)")
			} else {
				fmt.Printf("  %s\n", version)
			}
		}
	}
	fmt.Println("\nTo install a new version: govm install <version>")
	fmt.Println("To switch versions: govm use <version>")
}

// findMatchingVersion looks up a Go version available on go.dev.
// It first checks for an exact match, then falls back to the highest
// version that starts with query (with or without a separating dot).
func findMatchingVersion(version string) (utils.GoVersion, error) {
	msg := utils.FetchGoVersions()
	versions, ok := msg.(utils.VersionsMsg)
	if !ok {
		if errMsg, isErr := msg.(utils.ErrMsg); isErr {
			return utils.GoVersion{}, fmt.Errorf("failed to fetch versions: %v", errMsg)
		}
		return utils.GoVersion{}, fmt.Errorf("failed to fetch versions")
	}

	query := utils.NormalizeGoVersionQuery(version)
	versionStrings := make([]string, len(versions))
	for i, v := range versions {
		versionStrings[i] = v.Version
	}
	matched, ok := utils.FindLatestGoVersion(versionStrings, query)
	if !ok {
		return utils.GoVersion{}, fmt.Errorf("no version matching '%s' found", version)
	}
	for _, v := range versions {
		if v.Version == matched {
			return v, nil
		}
	}
	return utils.GoVersion{}, fmt.Errorf("no version matching '%s' found", version)
}

// findInstalledVersion mirrors findMatchingVersion but reads the
// installed govm versions directly from disk so the CLI works
// without contacting go.dev.
func findInstalledVersion(version string) (utils.GoVersion, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return utils.GoVersion{}, fmt.Errorf("failed to get home directory: %v", err)
	}
	goVersionsDir := filepath.Join(homeDir, ".govm", "versions")

	query := utils.NormalizeGoVersionQuery(version)
	exactPath := filepath.Join(goVersionsDir, "go"+query)
	if _, err := os.Stat(exactPath); err == nil {
		return utils.GoVersion{
			Version:   query,
			Path:      exactPath,
			Installed: true,
		}, nil
	}

	entries, err := os.ReadDir(goVersionsDir)
	if err != nil {
		return utils.GoVersion{}, fmt.Errorf("failed to read versions directory: %v", err)
	}

	var versions []string
	var paths []string
	versionToPath := make(map[string]string)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "go") {
			continue
		}
		versionStr := strings.TrimPrefix(entry.Name(), "go")
		versions = append(versions, versionStr)
		paths = append(paths, filepath.Join(goVersionsDir, entry.Name()))
		versionToPath[versionStr] = paths[len(paths)-1]
	}

	matched, ok := utils.FindLatestGoVersion(versions, query)
	if !ok {
		return utils.GoVersion{}, fmt.Errorf("no installed version matching '%s' found", version)
	}
	return utils.GoVersion{
		Version:   matched,
		Path:      versionToPath[matched],
		Installed: true,
	}, nil
}

func DeleteVersion(version string) {
	fmt.Printf("🔍 Looking for installed Go version matching %s...\n", version)
	matchedVersion, err := findInstalledVersion(version)
	if err != nil {
		fmt.Printf("❌ %s\n", err)
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("❌ Failed to get home directory: %v\n", err)
		return
	}

	activeVersionFile := filepath.Join(homeDir, ".govm", "active_version")
	activeVersion := ""
	if versionBytes, err := os.ReadFile(activeVersionFile); err == nil {
		activeVersion = string(versionBytes)
	}

	if matchedVersion.Version == activeVersion {
		fmt.Printf("❌ Cannot delete active version. Switch to another version first using 'govm use'.\n")
		return
	}

	fmt.Printf("⚠️  Are you sure you want to delete Go %s? (y/N): ", matchedVersion.Version)
	var response string
	fmt.Scanln(&response)

	if strings.ToLower(response) != "y" {
		fmt.Println("🛑 Operation canceled.")
		return
	}

	fmt.Printf("🗑️  Deleting Go %s...\n", matchedVersion.Version)

	msg := utils.DeleteVersion(matchedVersion)()
	switch msg := msg.(type) {
	case utils.ErrMsg:
		fmt.Printf("❌ Failed to delete version: %v\n", msg)
	case utils.DeleteCompleteMsg:
		fmt.Printf("✅ Successfully deleted Go %s\n", matchedVersion.Version)
	}
}
