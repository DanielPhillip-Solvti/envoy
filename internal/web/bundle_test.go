package web

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/example/staccato/internal/platform"
)

func TestDownloadAgentBundle(t *testing.T) {
	// Create required directories for natsauth
	os.MkdirAll("secrets/agents", 0755)
	os.MkdirAll("nats", 0755)
	defer os.RemoveAll("secrets")
	defer os.RemoveAll("nats")

	// Create a dummy binary for the test
	os.WriteFile("agent", []byte("dummy binary"), 0755)
	defer os.Remove("agent")

	// Create a dummy platform key for EnsureBootstrap
	os.WriteFile("secrets/platform.nk", []byte("SU..."), 0600)
	os.WriteFile("secrets/agent.nk", []byte("SU..."), 0600)

	// Setup a minimal server
	state := platform.NewMemoryState(time.Now)
	srv := &Server{
		state: state,
		auth:  newAuthManager(false),
	}

	// Create a request with required params
	req := httptest.NewRequest("GET", "/agents/bundle?agentID=test-agent&repo=https://github.com/test/repo&token=test-token", nil)

	// Inject session
	session, _ := srv.auth.create(httptest.NewRecorder(), authUser{Login: "testuser"}, "test-token")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.ID})

	w := httptest.NewRecorder()

	// Handle the request
	srv.downloadAgentBundle(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify ZIP content
	body := w.Body.Bytes()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	expectedFiles := []string{
		"staccato-agent",
		"agent.nk",
		"agent.env",
		"agent.manifest.yaml",
		"README.md",
	}

	fileMap := make(map[string]bool)
	for _, f := range zr.File {
		fileMap[f.Name] = true

		if f.Name == "agent.env" {
			rc, _ := f.Open()
			buf := new(bytes.Buffer)
			buf.ReadFrom(rc)
			rc.Close()
			content := buf.String()
			if !strings.Contains(content, "STACCATO_GIT_TOKEN=test-token") {
				t.Errorf("agent.env missing git token")
			}
		}
	}

	for _, ef := range expectedFiles {
		if !fileMap[ef] {
			t.Errorf("missing expected file in bundle: %s", ef)
		}
	}
}
