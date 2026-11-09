// Command tildoc-text extracts text for tildoc.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type handler func(ctx context.Context, c *client, j job, path, dir string) (results, error)

func worker(ctx context.Context, c *client, h handler, name string) {
	for ctx.Err() == nil {
		jobs, err := c.claim(ctx, 1)
		if err != nil {
			slog.Warn("claim", "plugin", name, "err", err)
		}
		if len(jobs) == 0 {
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
			continue
		}
		for _, j := range jobs {
			handle(ctx, c, h, j)
		}
	}
}

func handle(ctx context.Context, c *client, h handler, j job) {
	dir, err := os.MkdirTemp("", "tildoc-text-*")
	if err != nil {
		slog.Error("temp dir", "err", err)
		return
	}
	defer os.RemoveAll(dir)
	path := dir + "/in"
	r, err := func() (results, error) {
		if err := c.fetch(ctx, j, path); err != nil {
			return results{}, err
		}
		return h(ctx, c, j, path, dir)
	}()
	if ctx.Err() != nil {
		return // shutting down: the lease runs out and the job comes back
	}
	if err != nil {
		slog.Warn("job failed", "plugin", j.Plugin, "sha256", j.SHA256, "err", err)
		if err := c.fail(ctx, j, err.Error()); err != nil {
			slog.Error("report failure", "err", err)
		}
		return
	}
	if err := c.complete(ctx, j, r); err != nil {
		slog.Error("complete", "plugin", j.Plugin, "err", err)
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	base := os.Getenv("TILDOC_URL")
	token := os.Getenv("TILDOC_TEXT_TOKEN")
	if base == "" || token == "" {
		slog.Error("TILDOC_URL and TILDOC_TEXT_TOKEN are required")
		os.Exit(1)
	}
	n, err := strconv.Atoi(os.Getenv("TILDOC_TEXT_CONCURRENCY"))
	if err != nil || n < 1 {
		n = 4
	}
	httpc := &http.Client{Timeout: 30 * time.Minute}
	text := &client{base: base, token: token, http: httpc}
	var wg sync.WaitGroup
	h := func(ctx context.Context, _ *client, j job, path, _ string) (results, error) {
		return runText(ctx, j, path)
	}
	for range n {
		wg.Add(1)
		go func() { defer wg.Done(); worker(ctx, text, h, "tildoc-text") }()
	}
	slog.Info("tildoc-text started", "workers", n)
	wg.Wait()
}
