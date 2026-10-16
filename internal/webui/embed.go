package webui

import (
	"embed"
	"io/fs"
)

//go:embed public
var files embed.FS

func FS() fs.FS {
	sub, err := fs.Sub(files, "public")
	if err != nil {
		panic(err)
	}
	return sub
}
