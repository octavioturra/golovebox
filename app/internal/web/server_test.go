package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleHealth_returnsVMKey(t *testing.T) {
	srv := &Server{sb: nil} // sb=nil → Ready panics; use a stub
	// Use a no-op sandbox stub via the fakeSandbox helper below.
	srv.sb = fakeSandboxNotReady{}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	srv.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]healthResult
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body not JSON: %v — body: %s", err, w.Body)
	}
	res, ok := resp["vm"]
	if !ok {
		t.Fatal("response missing 'vm' key")
	}
	if res.OK {
		t.Fatal("expected ok=false for not-ready VM")
	}
}
