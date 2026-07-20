package model

import (
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/table"
	"github.com/smileoniks-ctrl/govm/internal/styles"
)

// tableStyles returns the table.Styles pair (header/selected/cell) for
// the given theme. It is called from Model.New and from
// applyRuntimeTheme when the theme changes.
func tableStyles(t styles.Theme) table.Styles {
	return table.Styles{
		Header:   t.TableHeaderStyle,
		Selected: t.TableSelectedStyle,
		Cell:     t.TableCellStyle,
	}
}

// listDefaultDelegate returns the bubbles/list delegate configured for
// the given theme. As with tableStyles, callers are Model.New and
// applyRuntimeTheme.
func listDefaultDelegate(t styles.Theme) list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = t.TableSelectedStyle
	delegate.Styles.SelectedDesc = t.TableSelectedStyle
	delegate.Styles.NormalDesc = t.MutedStyle
	return delegate
}
