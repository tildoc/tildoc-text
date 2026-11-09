package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPageStats(t *testing.T) {
	for _, c := range []struct {
		in           string
		pages, empty int
	}{
		{"enough text on this page\f", 1, 0},
		{"enough text on this page\f \f", 2, 1},
		{"\f\f", 2, 2},
	} {
		if p, e := pageStats(c.in); p != c.pages || e != c.empty {
			t.Errorf("%q: %d %d", c.in, p, e)
		}
	}
}

func TestLooksLikeText(t *testing.T) {
	if !looksLikeText([]byte("Grüße, plain text\n")) || looksLikeText([]byte{0, 1, 2}) || looksLikeText(nil) {
		t.Fatal("wrong classification")
	}
}

func TestPlainText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a")
	if err := os.WriteFile(path, []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := runText(context.Background(), job{MIME: "text/plain"}, path)
	if err != nil || r.Status != "done" || *r.Text != "hello world" {
		t.Fatalf("%+v %v", r, err)
	}
	if err := os.WriteFile(path, []byte{0, 1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	if r, _ := runText(context.Background(), job{MIME: "application/octet-stream"}, path); r.Status != "unsupported" {
		t.Fatalf("%+v", r)
	}
}
