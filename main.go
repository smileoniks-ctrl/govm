package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/smileoniks-ctrl/govm/internal/cli"
	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/model"
	"github.com/smileoniks-ctrl/govm/internal/services"
	"github.com/smileoniks-ctrl/govm/internal/setup"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
	"io"
	"os"
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
	settingsPath, settings := loadTUISettings(os.Stderr, func() (string, config.Settings, error) {
		path, s, _, err := config.LoadWithMigration()
		return path, s, err
	})
	runtime, err := services.NewRuntime(settings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing services: %v\n", err)
		return
	}
	app := cli.NewApp(cli.Operations{
		Runtime:      runtime,
		Install:      runtime.Install.Install,
		Activate:     runtime.Lifecycle.Activate,
		Delete:       runtime.Lifecycle.Delete,
		PreviewPrune: runtime.Prune.Preview,
		Prune:        runtime.Prune.Prune,
		Resolver:     runtime.Paths,
		ShimInPath:   utils.IsShimInPath,
	}, os.Stdin, os.Stdout, os.Stderr)
	if len(os.Args) > 1 {
		if handleCommandLine(app) != 0 {
			os.Exit(1)
		}
		return
	}
	// handleCommandLine and TUI should never throw at the same time
	launchTUI(runtime, settingsPath, settings)
}
func handleCommandLine(app *cli.App) int {
	if len(os.Args) < 2 {
		printUsage()
		return 0
	}
	command := os.Args[1]
	switch command {
	case "install":
		if len(os.Args) < 3 {
			fmt.Println("Error: 'install' requires a version argument")
			fmt.Println("Usage: govm install <version>")
			fmt.Println("Example: govm install 1.21")
			return 1
		}
		version := os.Args[2]
		version = strings.TrimPrefix(version, "go")
		app.InstallVersion(version)
	case "use":
		if len(os.Args) < 3 {
			fmt.Println("Error: 'use' requires a version argument")
			fmt.Println("Usage: govm use <version>")
			fmt.Println("Example: govm use 1.21")
			return 1
		}
		version := os.Args[2]
		version = strings.TrimPrefix(version, "go")
		app.UseVersion(version)
	case "delete":
		if len(os.Args) < 3 {
			fmt.Println("Error: 'delete' requires a version argument")
			fmt.Println("Usage: govm delete <version>")
			fmt.Println("Example: govm delete 1.21")
			return 1
		}
		version := os.Args[2]
		version = strings.TrimPrefix(version, "go")
		app.DeleteVersion(version)
	case "list":
		app.ListVersions()
	case "prune":
		if !app.PruneVersions(os.Args[2:]...) {
			return 1
		}
	case "deps":
		if len(os.Args) < 3 {
			app.DepsCommand("help")
			return 0
		}
		app.DepsCommand(os.Args[2:]...)
	case "help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		return 1
	}
	return 0
}
func printUsage() {
	fmt.Printf("GoVM - Go Version Manager (version %s)\n", utils.GetVersion())
	fmt.Println("\nUsage:")
	fmt.Println("  govm                   Launch the interactive TUI")
	fmt.Println("  govm install <version> Install a specific Go version")
	fmt.Println("  govm use <version>     Switch to a specific Go version")
	fmt.Println("  govm delete <version>  Delete a specific Go version")
	fmt.Println("  govm list              List installed Go versions")
	fmt.Println("  govm prune [options]   Remove inactive versions and temporary downloads")
	fmt.Println("  govm deps list         List current module dependencies")
	fmt.Println("  govm deps check        Check for available dependency updates")
	fmt.Println("  govm deps update       Update direct dependencies (interactive)")
	fmt.Println("  govm deps backups      List saved dependency backups")
	fmt.Println("  govm deps restore <file> Restore a saved dependency backup")
	fmt.Println("  govm help              Show this help message")
	fmt.Println("\nExamples:")
	fmt.Println("  govm install 1.21      Install Go 1.21.x (latest)")
	fmt.Println("  govm use 1.20          Switch to Go 1.20.x (latest)")
	fmt.Println("  govm deps update       Update direct deps in the current module")
}

func loadTUISettings(stderr io.Writer, load func() (string, config.Settings, error)) (string, config.Settings) {
	settingsPath, settings, err := load()
	if err != nil {
		fmt.Fprintf(stderr, "Warning: Failed to load settings: %v\n", err)
		return "", config.DefaultSettings()
	}
	return settingsPath, settings
}

func launchTUI(runtime *services.Runtime, settingsPath string, settings config.Settings) {
	shimInPath := utils.IsShimInPath()
	if !shimInPath {
		setupModel, err := setup.New()
		if err != nil {
			fmt.Printf("Error in setup: %v\n", err)
			return
		}
		p := tea.NewProgram(setupModel)
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error in setup: %v\n", err)
			os.Exit(1)
		}
	}
	shimPathWarning := ""
	if !shimInPath {
		shimPathWarning = "GoVM is not in your PATH. " + utils.GetShimPathInstructions()
	}
	theme := styles.NewTheme(config.ThemeName(settings.Theme))

	if _, err := os.UserHomeDir(); err != nil {
		fmt.Println("Error getting home directory:", err)
		os.Exit(1)
	}
	moduleDir, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting working directory:", err)
		os.Exit(1)
	}
	if _, err := runtime.Paths.VersionsDir(); err != nil {
		fmt.Println("Error resolving versions directory:", err)
		os.Exit(1)
	}

	initialModel := model.New(moduleDir, settingsPath, settings, shimPathWarning, theme).
		BindVersionOperations(model.VersionOperations{
			Runtime:             runtime,
			Install:             runtime.Install.Install,
			InstallWithProgress: runtime.Install.InstallWithProgress,
			Activate:            runtime.Lifecycle.Activate,
			Delete:              runtime.Lifecycle.Delete,
			ShimInPath:          utils.IsShimInPath,
			PreviewPrune:        runtime.Prune.Preview,
			Prune:               runtime.Prune.Prune,
			DiskUsage:           runtime.Prune.DiskUsage,
		})
	p := tea.NewProgram(
		model.NewProgramModel(initialModel),
		tea.WithFilter(model.FilterProgramMessage),
	)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
