package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	c, err := parseConfig()
	if err != nil {
		log.Error("配置无效", "error", err)
		os.Exit(2)
	}
	if c.selfCheck {
		if err = runSelfCheck(c.addr, log); err != nil {
			log.Error("自检失败", "error", err)
			os.Exit(1)
		}
		log.Info("自检通过")
		return
	}
	parts, err := assemble(c.database, log)
	if err != nil {
		log.Error("服务装配失败", "error", err)
		os.Exit(1)
	}
	defer parts.store.Close()
	srv := newHTTPServer(c.addr, parts.handler)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("服务关闭失败", "error", err)
		}
	}()
	log.Info("监测井封填见证台已启动", "addr", c.addr)
	if err = srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("HTTP 服务异常退出", "error", err)
		os.Exit(1)
	}
}
