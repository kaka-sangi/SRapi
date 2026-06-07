package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SearchResult is one web-search hit, fed back to the model and surfaced in the
// UI as a citation.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// SearchFunc runs a web search and returns ranked results. It is injected per
// turn by the handler (mirroring DispatchFunc) so the engine holds no provider
// config or secrets. A nil SearchFunc means web search is disabled.
type SearchFunc func(ctx context.Context, query string) ([]SearchResult, error)

const (
	maxSearchResults  = 5
	maxSnippetChars   = 600
	searchHTTPTimeout = 20 * time.Second
)

var searchHTTPClient = &http.Client{Timeout: searchHTTPTimeout}

// webSearchToolSchema is the OpenAI-function-shaped definition appended to the
// tool list when web search is configured for the turn.
func webSearchToolSchema() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        toolWebSearch,
			"description": "Search the public web for current information (news, docs, prices, anything outside this system). Returns titles, URLs, and snippets. Use it when the answer needs up-to-date or external knowledge, then cite the source URLs in your reply.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query.",
					},
				},
				"required": []any{"query"},
			},
		},
	}
}

// runWebSearch executes a web_search tool call and formats the results as a
// {data:[…]} JSON list — the model reads it, and the UI parses the same shape
// into a clickable sources list.
func runWebSearch(ctx context.Context, search SearchFunc, argsJSON string) (string, bool) {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(orEmptyJSON(argsJSON)), &args); err != nil {
		return "invalid web_search arguments: " + err.Error(), true
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "web_search requires a non-empty query", true
	}
	results, err := search(ctx, query)
	if err != nil {
		return "web search failed: " + err.Error(), true
	}
	if len(results) == 0 {
		return fmt.Sprintf(`{"query":%q,"data":[]}`, query), false
	}
	payload := map[string]any{"query": query, "data": results}
	out, err := json.Marshal(payload)
	if err != nil {
		return "failed to encode results: " + err.Error(), true
	}
	return string(out), false
}

// NewTavilySearch returns a SearchFunc backed by the Tavily Search API
// (purpose-built for LLM agents: clean answer + snippets).
func NewTavilySearch(apiKey, baseURL string) SearchFunc {
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if endpoint == "" {
		endpoint = "https://api.tavily.com"
	}
	return func(ctx context.Context, query string) ([]SearchResult, error) {
		body, _ := json.Marshal(map[string]any{
			"api_key":      apiKey,
			"query":        query,
			"max_results":  maxSearchResults,
			"search_depth": "basic",
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/search", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		raw, err := doSearchRequest(req, "tavily")
		if err != nil {
			return nil, err
		}
		var parsed struct {
			Results []struct {
				Title   string `json:"title"`
				URL     string `json:"url"`
				Content string `json:"content"`
			} `json:"results"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, fmt.Errorf("tavily: invalid response: %w", err)
		}
		out := make([]SearchResult, 0, len(parsed.Results))
		for _, r := range parsed.Results {
			out = append(out, SearchResult{Title: r.Title, URL: r.URL, Snippet: truncate(strings.TrimSpace(r.Content), maxSnippetChars)})
		}
		return out, nil
	}
}

// NewBraveSearch returns a SearchFunc backed by the Brave Search API (an
// independent web index).
func NewBraveSearch(apiKey, baseURL string) SearchFunc {
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if endpoint == "" {
		endpoint = "https://api.search.brave.com/res/v1/web/search"
	}
	return func(ctx context.Context, query string) ([]SearchResult, error) {
		full := endpoint + "?q=" + url.QueryEscape(query) + "&count=" + strconv.Itoa(maxSearchResults)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Subscription-Token", apiKey)
		raw, err := doSearchRequest(req, "brave")
		if err != nil {
			return nil, err
		}
		var parsed struct {
			Web struct {
				Results []struct {
					Title       string `json:"title"`
					URL         string `json:"url"`
					Description string `json:"description"`
				} `json:"results"`
			} `json:"web"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, fmt.Errorf("brave: invalid response: %w", err)
		}
		out := make([]SearchResult, 0, len(parsed.Web.Results))
		for _, r := range parsed.Web.Results {
			out = append(out, SearchResult{Title: r.Title, URL: r.URL, Snippet: truncate(strings.TrimSpace(r.Description), maxSnippetChars)})
		}
		return out, nil
	}
}

func doSearchRequest(req *http.Request, provider string) ([]byte, error) {
	resp, err := searchHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: request failed: %w", provider, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: search failed with HTTP %d", provider, resp.StatusCode)
	}
	return raw, nil
}
