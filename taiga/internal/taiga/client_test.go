package taiga

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListJSONSendsTaigaHeadersAndQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/projects" {
			t.Errorf("path = %q, want /api/v1/projects", r.URL.Path)
		}

		if got := r.URL.Query().Get("member"); got != "42" {
			t.Errorf("member = %q, want 42", got)
		}

		if got := r.Header.Get("Authorization"); got != "Application secret" {
			t.Errorf("authorization = %q, want Application secret", got)
		}

		if got := r.Header.Get("x-disable-pagination"); got != "True" {
			t.Errorf("x-disable-pagination = %q, want True", got)
		}

		w.Header().Set("x-paginated", "false")
		w.Header().Set("x-pagination-count", "2")
		w.Header().Set("Content-Type", "application/json")
		writeJSON(t, w, `[{"id":1},{"id":2}]`)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, Token: "secret", AuthScheme: "Application"})
	if err != nil {
		t.Fatal(err)
	}

	data, meta, err := client.ListJSON(context.Background(), "/projects", url.Values{"member": []string{"42"}}, false)
	if err != nil {
		t.Fatal(err)
	}

	items, ok := data.([]any)
	if !ok {
		t.Fatalf("data = %#v, want array", data)
	}

	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}

	if meta.Total != "2" {
		t.Fatalf("pagination total = %q, want 2", meta.Total)
	}
}

func TestNewClientNormalizesAPIPath(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "https://taiga.example.test/", Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	if got := client.endpointURL("/issues", nil).String(); got != "https://taiga.example.test/api/v1/issues" {
		t.Fatalf("endpoint = %q", got)
	}

	client, err = NewClient(Config{BaseURL: "https://taiga.example.test/api/v1/", Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	if got := client.endpointURL("/issues", nil).String(); got != "https://taiga.example.test/api/v1/issues" {
		t.Fatalf("endpoint with API path = %q", got)
	}
}

func TestDownloadUsesSafeUniqueFilenameAndLimit(t *testing.T) {
	downloadDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("authorization = %q, want Bearer secret", got)
		}

		w.Header().Set("Content-Type", "text/plain")
		writeJSON(t, w, "hello")
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:          server.URL,
		Token:            "secret",
		DownloadDir:      downloadDir,
		MaxDownloadBytes: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	fileURL := fmt.Sprintf("%s/file", server.URL)
	first, err := client.Download(context.Background(), fileURL, `../report:one.txt`, "")
	if err != nil {
		t.Fatal(err)
	}

	second, err := client.Download(context.Background(), fileURL, `../report:one.txt`, "")
	if err != nil {
		t.Fatal(err)
	}

	if first.Bytes != 5 {
		t.Fatalf("first download size = %d, want 5", first.Bytes)
	}

	if second.Bytes != 5 {
		t.Fatalf("second download size = %d, want 5", second.Bytes)
	}

	if filepath.Dir(first.Path) != downloadDir {
		t.Fatalf("first download escaped configured directory: %q", first.Path)
	}

	if filepath.Dir(second.Path) != downloadDir {
		t.Fatalf("second download escaped configured directory: %q", second.Path)
	}

	if filepath.Base(first.Path) == filepath.Base(second.Path) {
		t.Fatalf("downloads overwrote the same filename: %q", first.Path)
	}

	content, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}

	if string(content) != "hello" {
		t.Fatalf("download content = %q, want hello", content)
	}

	if strings.ContainsAny(filepath.Base(first.Path), `<>:"/\\|?*`) {
		t.Fatalf("unsafe filename was not sanitized: %q", filepath.Base(first.Path))
	}
}

func TestDownloadRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, "too large")
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:          server.URL,
		Token:            "secret",
		DownloadDir:      t.TempDir(),
		MaxDownloadBytes: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Download(context.Background(), fmt.Sprintf("%s/file", server.URL), "file.txt", "")
	if err == nil {
		t.Fatal("expected download limit error")
	}

	if !strings.Contains(err.Error(), "download limit") {
		t.Fatalf("download error = %v, want download limit error", err)
	}
}

func TestPatchJSONSendsMethodPathBodyAndAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q, want PATCH", r.Method)
		}

		if r.URL.Path != "/api/v1/userstories/42" {
			t.Errorf("path = %q, want /api/v1/userstories/42", r.URL.Path)
		}

		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("authorization = %q, want Bearer secret", got)
		}

		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q, want application/json", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}

		if strings.TrimSpace(string(body)) != `{"status":7}` {
			t.Errorf("body = %q, want {\"status\":7}", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		writeJSON(t, w, `{"id":42,"ref":5,"status":7}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}

	data, _, err := client.PatchJSON(context.Background(), "/userstories/42", nil, map[string]any{"status": 7})
	if err != nil {
		t.Fatal(err)
	}

	m, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want map", data)
	}

	id, ok := m["id"].(float64)
	if !ok {
		t.Fatalf("id = %#v, want float64", m["id"])
	}

	if int(id) != 42 {
		t.Fatalf("id = %v, want 42", m["id"])
	}
}

func TestPatchJSONSurfacesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"status": "não é um status válido"}`, http.StatusBadRequest)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = client.PatchJSON(context.Background(), "/userstories/42", nil, map[string]any{"status": 999})
	if err == nil {
		t.Fatal("expected HTTP 400 error")
	}

	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("PATCH error = %v, want HTTP 400 surface", err)
	}
}

func TestNewClientRequiresToken(t *testing.T) {
	_, err := NewClient(Config{BaseURL: "https://taiga.example.test"})
	if err == nil {
		t.Fatal("expected missing token error")
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Error(err)
	}
}
