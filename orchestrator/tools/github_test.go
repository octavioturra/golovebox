package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gogithub "github.com/google/go-github/v60/github"
	"github.com/user/golovebox/orchestrator/tools"
)

// newGitHubTestServer returns a test server simulating the GitHub API.
// handlers maps path prefixes to handler funcs.
func newGitHubTestServer(t *testing.T, mux *http.ServeMux) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestExecPR_createsWhenNoneExist(t *testing.T) {
	mux := http.NewServeMux()
	// List PRs — empty
	mux.HandleFunc("/api/v3/repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, []any{})
			return
		}
		// Create PR
		if r.Method == http.MethodPost {
			num := 42
			url := "https://github.com/owner/repo/pull/42"
			writeJSON(w, gogithub.PullRequest{Number: &num, HTMLURL: &url})
			return
		}
		http.NotFound(w, r)
	})
	srv := newGitHubTestServer(t, mux)

	// Patch ExecPR to use our test server.
	// Since go-github doesn't export a direct base URL setter on the token
	// client, we test via a helper that accepts a base URL.
	result, err := tools.ExecPRWithBase(context.Background(), "token", "owner", "repo",
		"feature/branch", "main", "My PR", "body", srv.URL+"/")
	if err != nil {
		t.Fatalf("ExecPR: %v", err)
	}
	if !strings.Contains(result, "42") {
		t.Fatalf("result = %q, want PR #42", result)
	}
}

func TestExecPR_updatesExistingOpenPR(t *testing.T) {
	mux := http.NewServeMux()
	num := 7
	url := "https://github.com/owner/repo/pull/7"
	mux.HandleFunc("/api/v3/repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, []gogithub.PullRequest{{Number: &num, HTMLURL: &url}})
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/v3/repos/owner/repo/pulls/7", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			writeJSON(w, gogithub.PullRequest{Number: &num, HTMLURL: &url})
			return
		}
		http.NotFound(w, r)
	})
	srv := newGitHubTestServer(t, mux)

	result, err := tools.ExecPRWithBase(context.Background(), "token", "owner", "repo",
		"feature/branch", "main", "My PR", "updated body", srv.URL+"/")
	if err != nil {
		t.Fatalf("ExecPR: %v", err)
	}
	if !strings.Contains(result, "7") {
		t.Fatalf("result = %q, want PR #7 update", result)
	}
}
