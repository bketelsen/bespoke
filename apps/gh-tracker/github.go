package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Item is one open PR or issue, cached as JSON in projects.cached_items.
type Item struct {
	Type      string `json:"type"` // "pr" or "issue"
	Title     string `json:"title"`
	URL       string `json:"url"`
	Number    int    `json:"number"`
	UpdatedAt string `json:"updated_at"` // RFC3339 UTC
}

type searchResponse struct {
	Items []struct {
		Title       string    `json:"title"`
		HTMLURL     string    `json:"html_url"`
		Number      int       `json:"number"`
		UpdatedAt   string    `json:"updated_at"`
		PullRequest *struct{} `json:"pull_request"`
	} `json:"items"`
}

// fetchOpenItems queries the GitHub search API for open PRs and issues in
// ownerRepo. token may be empty (unauthenticated, lower rate limit).
func fetchOpenItems(client *http.Client, ownerRepo, token string) ([]Item, error) {
	q := fmt.Sprintf("repo:%s is:open", ownerRepo)
	apiURL := "https://api.github.com/search/issues?per_page=50&q=" + url.QueryEscape(q)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api: %s", resp.Status)
	}

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(sr.Items))
	for _, it := range sr.Items {
		typ := "issue"
		if it.PullRequest != nil {
			typ = "pr"
		}
		items = append(items, Item{
			Type:      typ,
			Title:     it.Title,
			URL:       it.HTMLURL,
			Number:    it.Number,
			UpdatedAt: it.UpdatedAt,
		})
	}
	return items, nil
}

// httpClient is shared across fetches with a bounded timeout so a slow
// GitHub response never hangs a page load.
var httpClient = &http.Client{Timeout: 10 * time.Second}
