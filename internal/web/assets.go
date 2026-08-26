package web

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"

	"wellseal/internal/application"
)

//go:embed static/*
var assets embed.FS

type Handler struct {
	app    *application.Service
	log    *slog.Logger
	index  []byte
	static http.Handler
}

func New(app *application.Service, log *slog.Logger) (*Handler, error) {
	index, err := assets.ReadFile("static/index.html")
	if err != nil {
		return nil, err
	}
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	return &Handler{app: app, log: log, index: index, static: http.FileServer(http.FS(sub))}, nil
}

func (h *Handler) Index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(h.index)
}
