package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxTextBytes = 64 << 20
	minPageChars = 10 // below this a PDF page counts as having no text layer
)

// run executes a tool and keeps its output within bounds.
func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limited{buf: &stdout, max: 2 * maxTextBytes}
	cmd.Stderr = &limited{buf: &stderr, max: 64 << 10}
	cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1")
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.Bytes(), fmt.Errorf("%s: %s", filepath.Base(name), msg)
	}
	return stdout.Bytes(), nil
}

type limited struct {
	buf *bytes.Buffer
	max int
}

func (l *limited) Write(p []byte) (int, error) {
	if rem := l.max - l.buf.Len(); rem > 0 {
		if len(p) > rem {
			l.buf.Write(p[:rem])
		} else {
			l.buf.Write(p)
		}
	}
	return len(p), nil
}

func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	m, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	return buf[:m], nil
}

// looksLikeText accepts UTF-8 text without NUL bytes and few control characters.
func looksLikeText(b []byte) bool {
	if len(b) == 0 || bytes.IndexByte(b, 0) >= 0 {
		return false
	}
	for i := 0; i < 4 && !utf8.Valid(b) && len(b) > 0; i++ {
		b = b[:len(b)-1] // a rune cut at the end of the sample
	}
	if !utf8.Valid(b) {
		return false
	}
	ctrl := 0
	for _, r := range string(b) {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' && r != '\f' {
			ctrl++
		}
	}
	return ctrl*50 < len(b)
}

// pageStats splits pdftotext output and counts pages without a usable text layer.
func pageStats(text string) (pages, empty int) {
	parts := strings.Split(text, "\f")
	if len(parts) > 1 && strings.TrimSpace(parts[len(parts)-1]) == "" {
		parts = parts[:len(parts)-1]
	}
	for _, pg := range parts {
		n := 0
		for _, r := range pg {
			if !unicode.IsSpace(r) {
				n++
			}
		}
		if n < minPageChars {
			empty++
		}
	}
	return len(parts), empty
}

func pdfText(ctx context.Context, path string) (string, error) {
	out, err := run(ctx, "pdftotext", "-q", "-enc", "UTF-8", path, "-")
	return string(out), err
}

var pdfinfoPage = regexp.MustCompile(`(?m)^Page\s+(\d+)\s+size:\s+([\d.]+)\s+x\s+([\d.]+)\s+pts`)

// foreignOCR reports whether every page is a full-page image that also has a
// text layer: a scan that was recognised elsewhere.
func foreignOCR(ctx context.Context, path string, pages int) bool {
	if pages == 0 || pages > 5000 {
		return false
	}
	info, err := run(ctx, "pdfinfo", "-f", "1", "-l", strconv.Itoa(pages), path)
	if err != nil {
		return false
	}
	area := map[int]float64{}
	for _, m := range pdfinfoPage.FindAllStringSubmatch(string(info), -1) {
		n, _ := strconv.Atoi(m[1])
		w, _ := strconv.ParseFloat(m[2], 64)
		h, _ := strconv.ParseFloat(m[3], 64)
		area[n] = w * h
	}
	list, err := run(ctx, "pdfimages", "-list", path)
	if err != nil {
		return false
	}
	covered := map[int]bool{}
	for _, line := range strings.Split(string(list), "\n") {
		f := strings.Fields(line)
		if len(f) < 14 { // page num type width height color comp bpc enc interp object ID x-ppi y-ppi size ratio
			continue
		}
		page, err1 := strconv.Atoi(f[0])
		w, err2 := strconv.ParseFloat(f[3], 64)
		h, err3 := strconv.ParseFloat(f[4], 64)
		xppi, err4 := strconv.ParseFloat(f[12], 64)
		yppi, err5 := strconv.ParseFloat(f[13], 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || xppi <= 0 || yppi <= 0 {
			continue
		}
		if a := area[page]; a > 0 && (w/xppi*72)*(h/yppi*72) >= 0.8*a {
			covered[page] = true
		}
	}
	return len(area) == pages && len(covered) == pages
}
