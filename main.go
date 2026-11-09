// Command tildoc-text extracts text for tildoc.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

type handler func(ctx context.Context, c *client, j job, path, dir string) (results, error)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	base := os.Getenv("TILDOC_URL")
	if base == "" {
		slog.Error("TILDOC_URL is required")
		os.Exit(1)
	}
	listen := os.Getenv("PLUGIN_LISTEN")
	if listen == "" {
		listen = ":8000"
	}
	n, err := strconv.Atoi(os.Getenv("PLUGIN_CONCURRENCY"))
	if err != nil || n < 1 {
		n = 4
	}
	s := &server{ctx: ctx, client: &client{base: base, http: &http.Client{Timeout: 30 * time.Minute}},
		slots: make(chan struct{}, n), run: func(ctx context.Context, _ *client, j job, path, _ string) (results, error) {
			return runText(ctx, j, path)
		}, settings: nil}
	srv := &http.Server{Addr: listen, Handler: s.routes(), ReadHeaderTimeout: 30 * time.Second}
	go func() {
		<-ctx.Done()
		sd, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sd)
	}()
	slog.Info("tildoc-text started", "listen", listen, "workers", n)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("serve", "err", err)
		os.Exit(1)
	}
}
