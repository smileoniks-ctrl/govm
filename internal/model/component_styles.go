package model

import (
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/table"
	"github.com/smileoniks-ctrl/govm/internal/styles"
)

func tableStyles() table.Styles {
	return table.Styles{
		Header:   styles.TableHeaderStyle,
		Selected: styles.TableSelectedStyle,
		Cell:     styles.TableCellStyle,
	}
}

func listDefaultDelegate() list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = styles.TableSelectedStyle
	delegate.Styles.SelectedDesc = styles.TableSelectedStyle
	delegate.Styles.NormalDesc = styles.MutedStyle
	return delegate
}
