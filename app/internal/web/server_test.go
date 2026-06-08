package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/user/golovebox/app/internal/web"
)

// newTestStore creates a RunStore backed by a temp directory.
func newTestStore(t *testing.T) *web.RunStore {
	t.Helper()
	dir := t.TempDir()
	store, err := web.NewRunStore(dir)
	if err != nil {
		t.Fatalf("NewRunStore: %v", err)
	}
	return store
}

func TestHandleListRuns_emptyStoreReturnsArray(t *testing.T) {
	store := newTestStore(t)
	h := web.HandleListRuns(store)

	req := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var runs []any
	if err := json.Unmarshal(w.Body.Bytes(), &runs); err != nil {
		t.Fatalf("body not JSON array: %v — body: %s", err, w.Body)
	}
}

func TestHandleGetRun_missingRunReturns404(t *testing.T) {
	store := newTestStore(t)

	r := chi.NewRouter()
	r.Get("/api/runs/{id}", web.HandleGetRun(store))

	req := httptest.NewRequest(http.MethodGet, "/api/runs/no-such-run", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleGetRun_existingRunWithoutDAG(t *testing.T) {
	store := newTestStore(t)
	// Create run directory without dag.json — frontend must degrade gracefully.
	runDir := store.RunDir("run-abc")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "task.txt"), []byte("do something"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Get("/api/runs/{id}", web.HandleGetRun(store))

	req := httptest.NewRequest(http.MethodGet, "/api/runs/run-abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body not JSON: %v — body: %s", err, w.Body)
	}
	if resp["task"] != "do something" {
		t.Fatalf("task = %q, want %q", resp["task"], "do something")
	}
}
