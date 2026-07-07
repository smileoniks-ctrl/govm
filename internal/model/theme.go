package model

import "github.com/smileoniks-ctrl/govm/internal/styles"

func ApplyTheme(name styles.ThemeName) styles.ThemeName {
	applied := styles.ApplyTheme(name)
	rebuildDialogStyles()
	return applied
}
