package copilot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func TestTavilySearchNormalizes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"T1","url":"https://a","content":"c1"},{"title":"T2","url":"https://b","content":"c2"}]}`))
	}))
	defer srv.Close()

	res, err := NewTavilySearch("k", srv.URL)(context.Background(), "hi")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res) != 2 || res[0].Title != "T1" || res[0].URL != "https://a" || res[0].Snippet != "c1" {
		t.Fatalf("unexpected results: %+v", res)
	}
}

func TestBraveSearchNormalizes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "k" {
			t.Errorf("missing subscription token")
		}
		if r.URL.Query().Get("q") != "hi" {
			t.Errorf("unexpected query %q", r.URL.Query().Get("q"))
		}
		_, _ = w.Write([]byte(`{"web":{"results":[{"title":"B1","url":"https://x","description":"d1"}]}}`))
	}))
	defer srv.Close()

	res, err := NewBraveSearch("k", srv.URL)(context.Background(), "hi")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res) != 1 || res[0].Title != "B1" || res[0].Snippet != "d1" {
		t.Fatalf("unexpected results: %+v", res)
	}
}

func TestSearchReportsUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	if _, err := NewTavilySearch("bad", srv.URL)(context.Background(), "hi"); err == nil {
		t.Fatalf("expected an error on HTTP 401")
	}
}

func TestRunWebSearchFormatsDataList(t *testing.T) {
	search := func(_ context.Context, _ string) ([]SearchResult, error) {
		return []SearchResult{{Title: "T", URL: "https://u", Snippet: "s"}}, nil
	}
	content, isErr := runWebSearch(context.Background(), search, `{"query":"hi"}`)
	if isErr {
		t.Fatalf("unexpected error: %s", content)
	}
	if !strings.Contains(content, `"data"`) || !strings.Contains(content, "https://u") {
		t.Fatalf("content missing data list: %s", content)
	}
}

func TestRunWebSearchEmptyQueryErrors(t *testing.T) {
	if _, isErr := runWebSearch(context.Background(), func(context.Context, string) ([]SearchResult, error) {
		return nil, nil
	}, `{"query":"  "}`); !isErr {
		t.Fatalf("expected an error for an empty query")
	}
}

func TestEngineOffersAndRunsWebSearch(t *testing.T) {
	eng := NewEngine(mustCatalog(t))
	var toolNames []string
	searched := ""
	llm := func(_ context.Context, _ string, _ []provideradaptercontract.ConversationMessage, tools []map[string]any, _ func(string, string)) (provideradaptercontract.ConversationResponse, error) {
		if len(toolNames) == 0 {
			for _, tl := range tools {
				if fn, ok := tl["function"].(map[string]any); ok {
					toolNames = append(toolNames, fmt.Sprint(fn["name"]))
				}
			}
			return toolUseResponse("s1", toolWebSearch, `{"query":"golang news"}`), nil
		}
		return textResponse("Here's what I found."), nil
	}
	search := func(_ context.Context, q string) ([]SearchResult, error) {
		searched = q
		return []SearchResult{{Title: "T", URL: "https://u", Snippet: "s"}}, nil
	}
	var events []Event
	_, err := eng.Run(context.Background(), Settings{Enabled: true, AutoRunReads: true, MaxSteps: 8},
		[]Message{{Role: RoleUser, Content: "search the web"}}, nil, llm,
		func(context.Context, string, string, []byte) (int, []byte, error) {
			t.Fatal("web_search must not dispatch an admin call")
			return 0, nil, nil
		},
		search, func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if searched != "golang news" {
		t.Fatalf("web search not invoked with the query, got %q", searched)
	}
	found := false
	for _, n := range toolNames {
		if n == toolWebSearch {
			found = true
		}
	}
	if !found {
		t.Fatalf("web_search tool not offered to the model: %v", toolNames)
	}
	wantContains(t, collectTypes(events), EventToolResult)
}

func TestEngineOmitsWebSearchWhenNil(t *testing.T) {
	eng := NewEngine(mustCatalog(t))
	var toolNames []string
	llm := func(_ context.Context, _ string, _ []provideradaptercontract.ConversationMessage, tools []map[string]any, _ func(string, string)) (provideradaptercontract.ConversationResponse, error) {
		for _, tl := range tools {
			if fn, ok := tl["function"].(map[string]any); ok {
				toolNames = append(toolNames, fmt.Sprint(fn["name"]))
			}
		}
		return textResponse("ok"), nil
	}
	_, err := eng.Run(context.Background(), Settings{Enabled: true, AutoRunReads: true, MaxSteps: 8},
		[]Message{{Role: RoleUser, Content: "hi"}}, nil, llm,
		func(context.Context, string, string, []byte) (int, []byte, error) { return 0, nil, nil },
		nil, func(Event) {})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, n := range toolNames {
		if n == toolWebSearch {
			t.Fatalf("web_search tool must not be offered when search is nil")
		}
	}
}
