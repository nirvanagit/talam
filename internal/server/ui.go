package server

import (
	"io/fs"
	"net/http"
)

// uiHandler serves the embedded dashboard (internal/server/ui/dist). It's a
// single static HTML file with no build step — vanilla JS talking to the
// /v1/* API — so `go build` alone produces a working management UI with no
// npm toolchain required to run locally.
func uiHandler() http.Handler {
	sub, err := fs.Sub(uiFS, "ui/dist")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
