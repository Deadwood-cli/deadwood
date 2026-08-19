package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	gh "github.com/google/go-github/v63/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergedPRsByHeadWhenBudgetAllows(t *testing.T) {
	t.Parallel()

	var listedWithHead int
	srv := githubAPIServer(t, 5000, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("head") == "octo:feature-x" {
			listedWithHead++
			writeJSON(w, []map[string]any{
				prJSON(42, "feature-x", "2026-03-10T09:30:00Z"),
			})
			return
		}
		http.NotFound(w, r)
	})

	got, err := MergedPRs(context.Background(), testClient(t, srv), Repo{Owner: "octo", Name: "deadwood"}, []string{"feature-x"})

	require.NoError(t, err)
	require.Equal(t, 1, listedWithHead)
	match := got["feature-x"]
	assert.True(t, match.Found)
	assert.True(t, match.Merged)
	assert.Equal(t, 42, match.Number)
}

func TestMergedPRsFallsBackToClosedListWhenRateLimitIsLow(t *testing.T) {
	t.Parallel()

	var headQueries, unfiltered int
	srv := githubAPIServer(t, 2, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("head") != "" {
			headQueries++
			writeJSON(w, []any{})
			return
		}
		unfiltered++
		writeJSON(w, []map[string]any{
			prJSON(7, "gone-branch", "2026-01-02T00:00:00Z"),
			prJSON(8, "other", ""),
		})
	})

	got, err := MergedPRs(context.Background(), testClient(t, srv), Repo{Owner: "octo", Name: "deadwood"}, []string{"gone-branch", "other", "unrelated"})

	require.NoError(t, err)
	assert.Zero(t, headQueries, "must not do one call per branch when the budget is too low")
	assert.Equal(t, 1, unfiltered)
	assert.True(t, got["gone-branch"].Merged)
	assert.Equal(t, 7, got["gone-branch"].Number)
	assert.True(t, got["other"].Found)
	assert.False(t, got["other"].Merged)
	assert.False(t, got["unrelated"].Found)
}

func TestRepoDefaultBranch(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/octo/deadwood", r.URL.Path)
		writeJSON(w, map[string]string{"default_branch": "main"})
	}))
	t.Cleanup(srv.Close)

	got, err := RepoDefaultBranch(context.Background(), testClient(t, srv), Repo{Owner: "octo", Name: "deadwood"})

	require.NoError(t, err)
	assert.Equal(t, "main", got)
}

func testClient(t *testing.T, srv *httptest.Server) *gh.Client {
	t.Helper()
	return NewClient("test-token", ClientOptions{HTTP: srv.Client(), BaseURL: srv.URL})
}

func githubAPIServer(t *testing.T, remaining int, pulls http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rate_limit", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"resources":{"core":{"limit":5000,"remaining":` + strconv.Itoa(remaining) + `,"reset":4102444800},"search":{"limit":30,"remaining":30,"reset":4102444800}}}`))
	})
	mux.HandleFunc("/repos/octo/deadwood/pulls", pulls)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func prJSON(number int, head, mergedAt string) map[string]any {
	pr := map[string]any{
		"number": number,
		"head":   map[string]any{"ref": head},
	}
	if mergedAt != "" {
		pr["merged_at"] = mergedAt
	}
	return pr
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
