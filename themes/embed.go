// Package themes embeds the built-in themes into the kvist binary.
package themes

import (
	"embed"
	"io/fs"
)

//go:embed all:garden
var builtin embed.FS

// Builtin returns a built-in theme by name, or nil.
func Builtin(name string) fs.FS {
	sub, err := fs.Sub(builtin, name)
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "theme.toml"); err != nil {
		return nil
	}
	return sub
}
