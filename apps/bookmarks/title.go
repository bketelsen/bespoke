package main

import (
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var titleTagRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// fetchTitle GETs a page and extracts its <title>, best-effort. Empty
// string on any failure — callers fall back to the raw URL.
func fetchTitle(rawURL string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; bespoke-bookmarks/1.0)")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return ""
	}
	m := titleTagRe.FindSubmatch(body)
	if m == nil {
		return ""
	}
	title := html.UnescapeString(strings.TrimSpace(string(m[1])))
	title = strings.Join(strings.Fields(title), " ") // collapse whitespace/newlines
	if len(title) > 200 {
		title = title[:200]
	}
	return title
}
