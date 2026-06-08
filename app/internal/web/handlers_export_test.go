// handlers_export_test.go exposes internal handler logic for black-box tests.
// This file is compiled only when running tests.
package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

// HandleListRuns returns an http.HandlerFunc for GET /api/runs backed by store.
func HandleListRuns(store *RunStore) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		runs, _ := store.ListRuns()
		if runs == nil {
			runs = []RunMeta{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(runs)
	}
}

// HandleGetRun returns an http.HandlerFunc for GET /api/runs/{id} backed by store.
func HandleGetRun(store *RunStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := chi.URLParam(r, "id")
		if _, err := os.Stat(store.RunDir(runID)); os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		d, err := store.LoadDAG(runID)
		var dagJSON json.RawMessage
		if err == nil {
			dagJSON, _ = d.ToJSON()
		}
		statesData, _ := os.ReadFile(filepath.Join(store.RunDir(runID), "node_states.json"))
		var nodeStates any
		_ = json.Unmarshal(statesData, &nodeStates)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dag":            dagJSON,
			"node_states":    nodeStates,
			"task":           store.ReadTask(runID),
			"current_branch": store.GetRunMeta(runID, "current_branch"),
		})
	}
}
