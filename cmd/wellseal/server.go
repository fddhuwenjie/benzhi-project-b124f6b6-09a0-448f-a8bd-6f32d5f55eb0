package main

import (
	"log/slog"
	"net/http"
	"time"

	"wellseal/internal/application"
	"wellseal/internal/archive"
	"wellseal/internal/store"
	"wellseal/internal/web"
)

type assembled struct {
	store   *store.Store
	handler http.Handler
}

func assemble(database string, log *slog.Logger) (assembled, error) {
	s, err := store.Open(database)
	if err != nil {
		return assembled{}, err
	}
	app := application.New(s, archive.NewBuilder())
	wh, err := web.New(app, log)
	if err != nil {
		s.Close()
		return assembled{}, err
	}
	return assembled{store: s, handler: wh.Routes()}, nil
}
func newHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
}
