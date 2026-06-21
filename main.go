package main

import (
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/smileoniks-ctrl/govm/internal/cli"
	"github.com/smileoniks-ctrl/govm/internal/model"
	"github.com/smileoniks-ctrl/govm/internal/setup"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	// Check if user is requesting version information
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("govm %s\n", utils.GetVersion())
		os.Exit(0)
	}
	if err := utils.SetupShimDirectory(); err != nil {
		fmt.Printf("Warning: Failed to set up shim directory: %v\n", err)
	}
	if len(os.Args) > 1 {
		handleCommandLine()
		return
	}
	// handleCommandLine and TUI should never throw at the same time
	launchTUI()
}
func handleCommandLine() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}
	command := os.Args[1]
	switch command {
	case "install":
		if len(os.Args) < 3 {
			fmt.Println("Error: 'install' requires a version argument")
			fmt.Println("Usage: govm install <version>")
			fmt.Println("Example: govm install 1.21")
			return
		}
		version := os.Args[2]
		version = strings.TrimPrefix(version, "go")
		cli.InstallVersion(version)
	case "use":
		if len(os.Args) < 3 {
			fmt.Println("Error: 'use' requires a version argument")
			fmt.Println("Usage: govm use <version>")
			fmt.Println("Example: govm use 1.21")
			return
		}
		version := os.Args[2]
		version = strings.TrimPrefix(version, "go")
		cli.UseVersion(version)
	case "delete":
		if len(os.Args) < 3 {
			fmt.Println("Error: 'delete' requires a version argument")
			fmt.Println("Usage: govm delete <version>")
			fmt.Println("Example: govm delete 1.21")
			return
		}
		version := os.Args[2]
		version = strings.TrimPrefix(version, "go")
		cli.DeleteVersion(version)
	case "list":
		cli.ListVersions()
	case "help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
	}
}
func printUsage() {
	fmt.Println("GoVM - Go Version Manager")
	fmt.Println("\nUsage:")
	fmt.Println("  govm                   Launch the interactive TUI")
	fmt.Println("  govm install <version> Install a specific Go version")
	fmt.Println("  govm use <version>     Switch to a specific Go version")
	fmt.Println("  govm delete <version>  Delete a specific Go version")
	fmt.Println("  govm list              List installed Go versions")
	fmt.Println("  govm help              Show this help message")
	fmt.Println("\nExamples:")
	fmt.Println("  govm install 1.21      Install Go 1.21.x (latest)")
	fmt.Println("  govm use 1.20          Switch to Go 1.20.x (latest)")
}
func launchTUI() {
	if !setup.IsShimInPath() {
		setupModel := setup.New()
		p := tea.NewProgram(setupModel)
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error in setup: %v\n", err)
			os.Exit(1)
		}
	}
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.SpinnerStyle
	columns := []table.Column{
		{Title: "Version", Width: 10},
		{Title: "Path", Width: 40},
		{Title: "Status", Width: 10},
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	t.SetStyles(table.Styles{
		Header:   styles.TableHeaderStyle,
		Selected: styles.TableSelectedStyle,
		Cell:     styles.TableCellStyle,
	})
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error getting home directory:", err)
		os.Exit(1)
	}
	moduleDir, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting working directory:", err)
		os.Exit(1)
	}
	goVersionsDir := filepath.Join(homeDir, ".govm", "versions")
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = styles.TableSelectedStyle
	delegate.Styles.SelectedDesc = styles.TableSelectedStyle
	delegate.Styles.NormalDesc = styles.MutedStyle
	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "Available Versions"
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)

	depsColumns := []table.Column{
		{Title: "Dependency", Width: 24},
		{Title: "Current", Width: 9},
		{Title: "Latest", Width: 9},
		{Title: "Status", Width: 10},
	}
	depTable := table.New(
		table.WithColumns(depsColumns),
		table.WithFocused(true),
		table.WithHeight(15),
	)
	depTable.SetStyles(table.Styles{
		Header:   styles.TableHeaderStyle,
		Selected: styles.TableSelectedStyle,
		Cell:     styles.TableCellStyle,
	})

	initialModel := model.Model{
		List:            l,
		Versions:        []utils.GoVersion{},
		Spinner:         s,
		Loading:         true,
		HomeDir:         homeDir,
		GoVersionsDir:   goVersionsDir,
		InstalledTable:  t,
		Layout:          styles.LayoutNormal,
		ModuleDir:       moduleDir,
		DependencyTable: depTable,
	}
	p := tea.NewProgram(initialModel)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
