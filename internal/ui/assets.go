package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/*
var embeddedAssets embed.FS

func AssetsHandler() http.Handler {
	assets, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(assets))
}
