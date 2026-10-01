package mcpserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"ntdsk.com/taiga/internal/taiga"
)

func TestRegisterBuildsToolSchemas(t *testing.T) {
	client, err := taiga.NewClient(taiga.Config{BaseURL: "https://taiga.example.test", Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	New(client).Register(server)
}

func TestChangeStatusCard(t *testing.T) {
	gotMethod := ""
	gotPath := ""
	gotBody := ""

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPatch {
			writeText(t, w, `{"id":123,"ref":5,"subject":"Testar","status_extra_info":{"name":"In Progress"},"version":3}`)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}

		gotMethod, gotPath, gotBody = r.Method, r.URL.Path, string(body)
		writeText(t, w, `{"id":123,"ref":5,"subject":"Testar","status_extra_info":{"name":"Ready"},"version":4}`)
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	s := New(client)
	cardID := 123
	res, _, err := s.changeStatus(context.Background(), nil, ChangeStatusInput{
		Entity: "card", ItemID: &cardID, StatusID: 7,
	})
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)

	for _, want := range []string{"✅ Card #5", "Em andamento → **Pronto**"} {
		if !strings.Contains(text, want) {
			t.Errorf("output não contém %q:\n%s", want, text)
		}
	}

	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}

	if gotPath != "/api/v1/userstories/123" {
		t.Errorf("path = %q, want /api/v1/userstories/123", gotPath)
	}

	if strings.TrimSpace(gotBody) != `{"status":7,"version":3}` {
		t.Errorf("PATCH body = %q", gotBody)
	}
}

func TestChangeStatusIssue(t *testing.T) {
	gotMethod := ""
	gotPath := ""
	gotBody := ""

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPatch {
			writeText(t, w, `{"id":77,"ref":9,"subject":"Bug no login","status_extra_info":{"name":"New"},"version":1}`)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}

		gotMethod, gotPath, gotBody = r.Method, r.URL.Path, string(body)
		writeText(t, w, `{"id":77,"ref":9,"subject":"Bug no login","status_extra_info":{"name":"Ready"},"version":2}`)
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	s := New(client)
	issueID := 77
	res, _, err := s.changeStatus(context.Background(), nil, ChangeStatusInput{
		Entity: "issue", ItemID: &issueID, StatusID: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "✅ Issue #9") {
		t.Errorf("output não contém issue:\n%s", text)
	}

	if !strings.Contains(text, "Novo → **Pronto**") {
		t.Errorf("output não contém transição de status:\n%s", text)
	}

	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}

	if gotPath != "/api/v1/issues/77" {
		t.Errorf("path = %q, want /api/v1/issues/77", gotPath)
	}

	if strings.TrimSpace(gotBody) != `{"status":3,"version":1}` {
		t.Errorf("PATCH body = %q", gotBody)
	}
}

func TestSearch(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v1/userstories":
			writeText(t, w, `[{"id":1,"ref":10,"subject":"Corrigir bug no login","status_extra_info":{"name":"Ready"},"assigned_to_extra_info":{},"project_extra_info":{"name":"App Mobile"}}]`)
		case "/api/v1/issues":
			writeText(t, w, `[{"id":2,"ref":11,"subject":"Melhorar performance","status":5,"project_extra_info":{"name":"Plataforma Web"}}]`)
		case "/api/v1/tasks":
			writeText(t, w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	s := New(client)

	res, _, err := s.search(context.Background(), nil, SearchInput{Query: "bug"})
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	for _, want := range []string{"1 resultado", "Card", "Corrigir bug no login", "App Mobile", "| Projeto |"} {
		if !strings.Contains(text, want) {
			t.Errorf("search output não contém %q:\n%s", want, text)
		}
	}

	res, _, err = s.search(context.Background(), nil, SearchInput{Query: "inexistente"})
	if err != nil {
		t.Fatal(err)
	}

	text = resultText(t, res)
	if !strings.Contains(text, "Nenhum resultado") {
		t.Errorf("search vazio inesperado:\n%s", text)
	}
}

func TestAttachmentEndpoint(t *testing.T) {
	tests := []struct {
		entity   string
		expected string
	}{
		{entity: "userstory", expected: "/userstories"},
		{entity: "card", expected: "/userstories"},
		{entity: "issue", expected: "/issues"},
		{entity: "task", expected: "/tasks"},
		{entity: "subtask", expected: "/tasks"},
	}

	for _, test := range tests {
		got, err := attachmentEndpoint(test.entity)
		if err != nil {
			t.Fatalf("attachmentEndpoint(%q) error: %v", test.entity, err)
		}

		if got != test.expected {
			t.Fatalf("attachmentEndpoint(%q) = %q, want %q", test.entity, got, test.expected)
		}
	}

	_, err := attachmentEndpoint("epic")
	if err == nil {
		t.Fatal("expected invalid entity error")
	}
}

func TestPaging(t *testing.T) {
	query, paginated, err := paging(nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if paginated {
		t.Fatalf("paging without values = %v, want false", paginated)
	}

	if len(query) != 0 {
		t.Fatalf("paging without values query = %#v, want empty", query)
	}

	page := 2
	pageSize := 50
	query, paginated, err = paging(&page, &pageSize)
	if err != nil {
		t.Fatal(err)
	}

	if !paginated {
		t.Fatal("paging = false, want true")
	}

	if query.Get("page") != "2" {
		t.Errorf("page = %q, want 2", query.Get("page"))
	}

	if query.Get("page_size") != "50" {
		t.Errorf("page_size = %q, want 50", query.Get("page_size"))
	}

	_, _, err = paging(nil, intPointer(1001))
	if err == nil {
		t.Fatal("expected page size validation error")
	}
}

func TestProjectSlugQueryIsEscaped(t *testing.T) {
	query := url.Values{"slug": []string{"project with spaces"}}
	if got := query.Encode(); got != "slug=project+with+spaces" {
		t.Fatalf("encoded query = %q", got)
	}
}

func intPointer(value int) *int { return &value }

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()

	content, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content = %#v, want TextContent", res.Content[0])
	}

	return content.Text
}

func writeText(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Error(err)
	}
}
