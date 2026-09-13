package model

// This file is the single keybinding registry (see docs/adr/0001):
// every key hint the TUI shows — the one-line hint bar at the bottom
// of the screen and the Help overlay opened with "?" — renders from
// the sections defined here. Adding or changing a binding means
// changing this file and nothing else.
//
// The registry lists only bindings the key-dispatch code actually
// accepts: handleKey, handleDialogKey, handleSettingsKey, or
// depsDialog.Handle must recognise every documented key.

// keyBinding is one entry in the keybinding registry: the keys as
// displayed to the user, what they do, and whether the entry also
// appears in the one-line hint bar.
type keyBinding struct {
	keys  string
	desc  string
	short bool
}

// helpSection is a titled group of bindings. Tab sections, dialog
// sections, and confirmation sections carry the bindings of one
// input context; the global sections list bindings that work across
// a whole context family.
type helpSection struct {
	title    string
	bindings []keyBinding
}

// globalKeyBindings lists the keys that work on every tab. They are
// appended to every tab's Help overlay and (when short) to every
// tab's hint bar. Tab is overlay-only: with the Available filter key
// in the bar, showing "tab next tab" too would push the quit hint out
// of an 80-column terminal, and Tab navigation is standard TUI
// convention the overlay still documents.
func globalKeyBindings() helpSection {
	return helpSection{
		title: "Global",
		bindings: []keyBinding{
			{keys: "tab", desc: "next tab"},
			{keys: "shift+tab", desc: "previous tab"},
			{keys: "?", desc: "help", short: true},
			{keys: "q / ctrl+c", desc: "quit", short: true},
		},
	}
}

// dialogGlobalKeyBindings lists the keys that work while any modal
// dialog or confirmation is open. Tab and Shift+Tab are deliberately
// absent: inside dialogs they toggle the highlighted choice instead
// of switching tabs.
func dialogGlobalKeyBindings() helpSection {
	return helpSection{
		title: "Global",
		bindings: []keyBinding{
			{keys: "?", desc: "help", short: true},
			{keys: "q / ctrl+c", desc: "quit", short: true},
		},
	}
}

// tabKeyBindings returns the section describing the bindings of one
// tab. Cursor movement is overlay-only on the list/table tabs (the
// hint bar keeps it on Settings, which has few other actions).
func tabKeyBindings(tab int) helpSection {
	move := keyBinding{keys: "↑/↓ k/j", desc: "move cursor"}
	switch tab {
	case AvailableTab:
		return helpSection{
			title: "Available",
			bindings: []keyBinding{
				{keys: "i", desc: "install", short: true},
				{keys: "u", desc: "use", short: true},
				{keys: "d", desc: "delete", short: true},
				{keys: "r", desc: "refresh", short: true},
				{keys: "f", desc: "find", short: true},
				move,
			},
		}
	case InstalledTab:
		return helpSection{
			title: "Installed",
			bindings: []keyBinding{
				{keys: "u", desc: "use", short: true},
				{keys: "d", desc: "delete", short: true},
				{keys: "p", desc: "prune", short: true},
				move,
			},
		}
	case DepsTab:
		return helpSection{
			title: "Deps",
			bindings: []keyBinding{
				{keys: "r", desc: "check updates", short: true},
				{keys: "space", desc: "mark", short: true},
				{keys: "a", desc: "mark all / none", short: true},
				{keys: "u", desc: "update", short: true},
				{keys: "b", desc: "backups", short: true},
				move,
			},
		}
	case SettingsTab:
		return helpSection{
			title: "Settings",
			bindings: []keyBinding{
				{keys: "↑/↓ k/j", desc: "move", short: true},
				{keys: "enter / space", desc: "toggle or edit", short: true},
				{keys: "←/→ h/l", desc: "toggle or adjust"},
			},
		}
	}
	return helpSection{title: "Tabs", bindings: nil}
}

// dialogKeyBindings returns the section describing the open
// dependency dialog. The restore dialog adds backup navigation and
// re-labels enter to the highlighted button, mirroring the hint bar.
func dialogKeyBindings(dialog depsDialog) helpSection {
	var bindings []keyBinding
	title := "Dialog"
	escDesc := "cancel"
	enterDesc := "confirm"

	switch dialog.kind {
	case dialogUpdate:
		title = "Update dependencies"
		bindings = append(bindings, keyBinding{keys: "↑/↓ k/j", desc: "level", short: true})
		if dialog.canToggleScope() {
			bindings = append(bindings, keyBinding{keys: "space", desc: "scope", short: true})
		}
	case dialogChecks:
		title = "Run checks"
		escDesc = "skip"
	case dialogRollback:
		title = "Roll back"
	case dialogRestore:
		title = "Restore backup"
		escDesc = "cancel"
		enterDesc = "cancel"
		if dialog.choiceYes {
			enterDesc = "restore"
		}
		bindings = append(bindings, keyBinding{keys: "↑/↓ k/j", desc: "select backup", short: true})
	}

	bindings = append(bindings,
		keyBinding{keys: "←/→ h/l", desc: "choose", short: true},
		keyBinding{keys: "enter", desc: enterDesc, short: true},
		keyBinding{keys: "y", desc: "accept"},
		keyBinding{keys: "n / esc", desc: escDesc, short: true},
	)
	return helpSection{title: title, bindings: bindings}
}

// confirmDeleteKeyBindings is the section shown while a deletion
// awaits its y/n confirmation.
func confirmDeleteKeyBindings() helpSection {
	return helpSection{
		title: "Confirm delete",
		bindings: []keyBinding{
			{keys: "y", desc: "confirm", short: true},
			{keys: "n", desc: "cancel", short: true},
		},
	}
}

// confirmPruneKeyBindings is the section shown while the prune dialog
// awaits its answer. It lists the same keys as the Deps dialogs.
func confirmPruneKeyBindings() helpSection {
	return helpSection{
		title: "Confirm prune",
		bindings: []keyBinding{
			{keys: "←/→ h/l", desc: "choose", short: true},
			{keys: "enter", desc: "confirm", short: true},
			{keys: "y", desc: "accept"},
			{keys: "n / esc", desc: "cancel", short: true},
		},
	}
}

// editingKeyBindings returns the bar-only section for the Settings
// text inputs. The Help overlay cannot open while an input has focus
// ("?" is ordinary input there), so these bindings never appear in
// the overlay.
func editingKeyBindings(editingSource bool) helpSection {
	if editingSource {
		return helpSection{
			title: "Edit distribution source",
			bindings: []keyBinding{
				{keys: "enter", desc: "check and save", short: true},
				{keys: "r", desc: "reset", short: true},
				{keys: "esc", desc: "cancel", short: true},
			},
		}
	}
	return helpSection{
		title: "Edit backup limit",
		bindings: []keyBinding{
			{keys: "enter", desc: "save", short: true},
			{keys: "esc", desc: "cancel", short: true},
		},
	}
}

// filterInputKeyBindings is the bar-only section shown while the
// Available list's filter input has focus. q and ? are deliberately
// absent: they are ordinary input characters while typing. The Help
// overlay cannot open in this state (? is ordinary input there), so these
// bindings never appear in the overlay.
func filterInputKeyBindings() helpSection {
	return helpSection{
		title: "Find input",
		bindings: []keyBinding{
			{keys: "enter", desc: "apply", short: true},
			{keys: "esc", desc: "clear", short: true},
			{keys: "tab", desc: "next tab", short: true},
			{keys: "ctrl+c", desc: "quit", short: true},
		},
	}
}

// helpOverlayBarBindings is the hint bar shown while the Help overlay
// itself is open. q is deliberately absent: while the overlay is open
// q is swallowed and only ctrl+c quits.
func helpOverlayBarBindings() helpSection {
	return helpSection{
		title: "Help overlay",
		bindings: []keyBinding{
			{keys: "? / esc", desc: "close help", short: true},
			{keys: "ctrl+c", desc: "quit", short: true},
		},
	}
}

// shortHints flattens the short-flagged bindings of the sections into
// the key/description pairs the hint bar renders.
func shortHints(sections []helpSection) [][2]string {
	var hints [][2]string
	for _, section := range sections {
		for _, binding := range section.bindings {
			if binding.short {
				hints = append(hints, [2]string{binding.keys, binding.desc})
			}
		}
	}
	return hints
}
