// web_search, gateway-implemented (ADR-0025): the runtime exposes no hosted
// search tool to SDK sessions, so the gateway serves the tool itself against
// the Brave Search API. Key from BESPOKE_BRAVE_API_KEY (or BRAVE_API_KEY);
// without it web_search degrades to fetch-based search (the webFetchHint).
package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	copilot "github.com/github/copilot-sdk/go"
)

var braveSearchURL = "https://api.search.brave.com/res/v1/web/search" // test seam

var braveClient = &http.Client{Timeout: 15 * time.Second}

func braveKey() string {
	k := cmp.Or(os.Getenv("BESPOKE_BRAVE_API_KEY"), os.Getenv("BRAVE_API_KEY"))
	if k == "" {
		log.Println("llm: no Brave key (set BESPOKE_BRAVE_API_KEY) — web_search degrades to fetch-based search")
	}
	return k
}

// braveSearchTool is a session-local custom tool, same shape as app tools
// but executed in-process — SkipPermission is ours by construction, exactly
// like copilotTools.
func braveSearchTool(key string) copilot.Tool {
	return copilot.Tool{
		Name: "web_search",
		Description: "Search the web. Returns titles, URLs, and snippets for the top results; " +
			"use web_fetch to read a promising result in full.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "The search query."},
			},
			"required": []string{"query"},
		},
		SkipPermission: true,
		Handler: func(inv copilot.ToolInvocation) (copilot.ToolResult, error) {
			raw, err := json.Marshal(inv.Arguments)
			if err != nil {
				return copilot.ToolResult{}, err
			}
			var a struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(raw, &a); err != nil || strings.TrimSpace(a.Query) == "" {
				return copilot.ToolResult{TextResultForLLM: "web_search needs a non-empty query", ResultType: "failure"}, nil
			}
			text, err := braveSearch(key, a.Query)
			if err != nil {
				// Failures go back to the model as text so it can fall back
				// to web_fetch or explain, not as session errors.
				log.Printf("llm-tool web_search err=%v", err)
				return copilot.ToolResult{TextResultForLLM: "search error: " + err.Error(), ResultType: "failure"}, nil
			}
			log.Printf("llm-tool web_search ok")
			return copilot.ToolResult{TextResultForLLM: text, ResultType: "success"}, nil
		},
	}
}

func braveSearch(key, query string) (string, error) {
	q := url.Values{"q": {query}, "count": {"8"}}
	req, err := http.NewRequest(http.MethodGet, braveSearchURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Subscription-Token", key)
	req.Header.Set("Accept", "application/json")
	resp, err := braveClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("brave search: %s: %.200s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var body struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", fmt.Errorf("brave search: %w", err)
	}
	if len(body.Web.Results) == 0 {
		return "No results for " + query, nil
	}
	var b strings.Builder
	for i, r := range body.Web.Results {
		fmt.Fprintf(&b, "%d. %s\n%s\n%s\n\n", i+1, r.Title, r.URL, r.Description)
	}
	return strings.TrimSpace(b.String()), nil
}
