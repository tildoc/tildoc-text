package main

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServerTakesAJobAndRepliesWithItsToken(t *testing.T) {
	got := make(chan string, 1)
	tildoc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.URL.Path + " " + r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer tildoc.Close()
	s := &server{ctx: context.Background(), client: &client{base: tildoc.URL, http: tildoc.Client()}, slots: make(chan struct{}, 1),
		run: func(context.Context, *client, job, string, string) (results, error) {
			return results{Status: "done"}, nil
		}}
	plugin := httptest.NewServer(s.routes())
	defer plugin.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	meta, _ := json.Marshal(job{ID: "abc", ReplyToken: "one-job"})
	_ = mw.WriteField("job", string(meta))
	part, _ := mw.CreateFormFile("file", "content")
	_, _ = part.Write([]byte("hello"))
	mw.Close()
	resp, err := http.Post(plugin.URL+"/jobs", mw.FormDataContentType(), &body)
	if err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("%v %v", resp, err)
	}
	select {
	case line := <-got:
		if !strings.Contains(line, "/jobs/abc/complete") || !strings.HasSuffix(line, "Bearer one-job") {
			t.Fatal(line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no reply")
	}
}

func TestServerSaysBusyWhenFull(t *testing.T) {
	s := &server{ctx: context.Background(), client: &client{}, slots: make(chan struct{}, 1)}
	s.slots <- struct{}{}
	plugin := httptest.NewServer(s.routes())
	defer plugin.Close()
	resp, err := http.Post(plugin.URL+"/jobs", "multipart/form-data; boundary=x", strings.NewReader(""))
	if err != nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("%v %v", resp, err)
	}
}
