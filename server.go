package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
)

// server takes job packages from tildoc. It holds no credential: each job
// brings the one token that delivers its result.
type server struct {
	ctx      context.Context
	client   *client
	slots    chan struct{}
	run      handler
	settings []setting
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /jobs", s.accept)
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"settings": s.settings})
	})
	return mux
}

func (s *server) accept(w http.ResponseWriter, r *http.Request) {
	select {
	case s.slots <- struct{}{}:
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	dir, err := os.MkdirTemp("", "tildoc-text-*")
	if err != nil {
		<-s.slots
		http.Error(w, "no temp space", http.StatusInternalServerError)
		return
	}
	j, path, err := receive(r, dir)
	if err != nil {
		os.RemoveAll(dir)
		<-s.slots
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	go func() {
		defer func() { <-s.slots }()
		defer os.RemoveAll(dir)
		s.work(j, path, dir)
	}()
}

// receive stores the package: the job, its text if sent, and the file.
func receive(r *http.Request, dir string) (job, string, error) {
	var j job
	path := filepath.Join(dir, "in")
	mr, err := r.MultipartReader()
	if err != nil {
		return j, "", err
	}
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return j, "", err
		}
		switch p.FormName() {
		case "job":
			if err := json.NewDecoder(io.LimitReader(p, 1<<20)).Decode(&j); err != nil {
				return j, "", err
			}
		case "text":
			b, err := io.ReadAll(p)
			if err != nil {
				return j, "", err
			}
			j.Text = string(b)
		case "file":
			f, err := os.Create(path)
			if err != nil {
				return j, "", err
			}
			if _, err := io.Copy(f, p); err != nil {
				f.Close()
				return j, "", err
			}
			if err := f.Close(); err != nil {
				return j, "", err
			}
		}
	}
	return j, path, nil
}

func (s *server) work(j job, path, dir string) {
	r, err := s.run(s.ctx, s.client, j, path, dir)
	if s.ctx.Err() != nil {
		return // shutting down: the lease runs out and the job comes back
	}
	if err != nil {
		slog.Warn("job failed", "plugin", j.Plugin, "sha256", j.SHA256, "err", err)
		if err := s.client.fail(s.ctx, j, err.Error()); err != nil {
			slog.Error("report failure", "err", err)
		}
		return
	}
	if err := s.client.complete(s.ctx, j, r); err != nil {
		slog.Error("complete", "plugin", j.Plugin, "err", err)
	}
}
