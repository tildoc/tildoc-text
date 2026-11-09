package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// job is one piece of work from tildoc.
type job struct {
	ID         string         `json:"id"`
	Plugin     string         `json:"plugin"`
	SHA256     string         `json:"sha256"`
	MIME       string         `json:"mime"`
	Force      bool           `json:"force"`
	VariantOf  string         `json:"variant_of"` // this content is a variant of that file
	Settings   map[string]any `json:"settings"`
	ReplyToken string         `json:"reply_token"`
	Text       string         `json:"-"` // the extracted text, if the plugin asked for it
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

// client delivers results to tildoc, with the token that came with each job.
type client struct {
	base string
	http *http.Client
}

func (c *client) do(ctx context.Context, token, method, path string, body io.Reader, ctype string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
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

func (c *client) postJSON(ctx context.Context, token, path string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return c.do(ctx, token, http.MethodPost, path, bytes.NewReader(b), "application/json", out)
}

func (c *client) complete(ctx context.Context, j job, r results) error {
	return c.postJSON(ctx, j.ReplyToken, "/api/v1/plugin/jobs/"+j.ID+"/complete", r, nil)
}

func (c *client) fail(ctx context.Context, j job, msg string) error {
	return c.postJSON(ctx, j.ReplyToken, "/api/v1/plugin/jobs/"+j.ID+"/fail", map[string]any{"error": msg, "retry": false}, nil)
}

// setting is one option shown on tildoc's Administration page; this plugin has none.
type setting struct{}
