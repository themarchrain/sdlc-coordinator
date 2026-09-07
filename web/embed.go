package adminweb

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var embeddedFiles embed.FS

func Handler() http.Handler {
	dist, err := fs.Sub(embeddedFiles, "dist")
	if err != nil {
		return NewHandler(emptyFS{})
	}
	return NewHandler(dist)
}

type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }
