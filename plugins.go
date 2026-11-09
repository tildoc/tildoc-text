package main

import (
	"context"
	"os"
	"strconv"
	"strings"
)

func textFact(kind string, pages int) []fact {
	f := []fact{{Label: "fact.kind", Value: "kind." + kind}}
	if pages > 0 {
		f = append(f, fact{Label: "fact.pages", Value: strconv.Itoa(pages)})
	}
	return f
}

// runText is the tildoc-text plugin.
func runText(ctx context.Context, j job, path string) (results, error) {
	head, err := readHead(path, 8192)
	if err != nil {
		return results{}, err
	}
	if j.MIME != "application/pdf" {
		if !looksLikeText(head) {
			return results{Status: "unsupported"}, nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return results{}, err
		}
		if len(b) > maxTextBytes {
			b = b[:maxTextBytes]
		}
		text := string(b)
		return results{Status: "done", Facts: textFact("text", 0), Text: &text}, nil
	}
	text, err := pdfText(ctx, path)
	if err != nil {
		return results{}, err
	}
	pages, empty := pageStats(text)
	kind := "pdf-digital"
	switch {
	case j.VariantOf != "":
		kind = "pdf-own"
	case empty == pages:
		kind = "pdf-scanned"
	case empty > 0:
		kind = "pdf-mixed"
	case foreignOCR(ctx, path, pages):
		kind = "pdf-ocred"
	}
	r := results{Status: "done", Facts: textFact(kind, pages)}
	if strings.TrimSpace(text) != "" {
		r.Text = &text
	}
	return r, nil
}
