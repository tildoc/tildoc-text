package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// job is one piece of work from tildoc.
type job struct {
	ID          string         `json:"id"`
	Plugin      string         `json:"plugin"`
	SHA256      string         `json:"sha256"`
	MIME        string         `json:"mime"`
	ContentPath string         `json:"content_path"`
	Force       bool           `json:"force"`
	VariantOf   string         `json:"variant_of"` // this content is a variant of that file
	Settings    map[string]any `json:"settings"`
}

type fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// results are what a job reports.
type results struct {
	Status string  `json:"status"` // done | partial | unsupported
	Facts  []fact  `json:"facts,omitempty"`
	Text   *string `json:"text,omitempty"`
}

// client talks to tildoc as one plugin.
type client struct {
	base, token string
	http        *http.Client
}

func (c *client) do(ctx context.Context, method, path string, body io.Reader, ctype string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, msg)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *client) postJSON(ctx context.Context, path string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, path, bytes.NewReader(b), "application/json", out)
}

func (c *client) claim(ctx context.Context, n int) ([]job, error) {
	var jobs []job
	err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/plugin/jobs/claim?limit=%d", n), nil, "", &jobs)
	return jobs, err
}

// fetch stores a job's file at path.
func (c *client) fetch(ctx context.Context, j job, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+j.ContentPath, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: %d", resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return f.Close()
}

func (c *client) complete(ctx context.Context, j job, r results) error {
	return c.postJSON(ctx, "/api/v1/plugin/jobs/"+j.ID+"/complete", r, nil)
}

func (c *client) fail(ctx context.Context, j job, msg string) error {
	return c.postJSON(ctx, "/api/v1/plugin/jobs/"+j.ID+"/fail", map[string]any{"error": msg, "retry": false}, nil)
}
