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
	var gotMethod, gotPath, gotBody string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			body, _ := io.ReadAll(r.Body)
			gotMethod, gotPath, gotBody = r.Method, r.URL.Path, string(body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":123,"ref":5,"subject":"Testar","status_extra_info":{"name":"Ready"},"version":4}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":123,"ref":5,"subject":"Testar","status_extra_info":{"name":"In Progress"},"version":3}`)
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

	content, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content = %#v, want TextContent", res.Content[0])
	}
	for _, want := range []string{"✅ Card #5", "Em andamento → **Pronto**"} {
		if !strings.Contains(content.Text, want) {
			t.Errorf("output não contém %q:\n%s", want, content.Text)
		}
	}
	if gotMethod != http.MethodPatch || gotPath != "/api/v1/userstories/123" {
		t.Errorf("PATCH = %s %s, want PATCH /api/v1/userstories/123", gotMethod, gotPath)
	}
	if strings.TrimSpace(gotBody) != `{"status":7,"version":3}` {
		t.Errorf("PATCH body = %q", gotBody)
	}
}

func TestChangeStatusIssue(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			body, _ := io.ReadAll(r.Body)
			gotMethod, gotPath, gotBody = r.Method, r.URL.Path, string(body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":77,"ref":9,"subject":"Bug no login","status_extra_info":{"name":"Ready"},"version":2}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":77,"ref":9,"subject":"Bug no login","status_extra_info":{"name":"New"},"version":1}`)
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
	content, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content = %#v, want TextContent", res.Content[0])
	}
	if !strings.Contains(content.Text, "✅ Issue #9") || !strings.Contains(content.Text, "Novo → **Pronto**") {
		t.Errorf("output inesperado:\n%s", content.Text)
	}
	if gotMethod != http.MethodPatch || gotPath != "/api/v1/issues/77" {
		t.Errorf("PATCH = %s %s, want PATCH /api/v1/issues/77", gotMethod, gotPath)
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
			_, _ = io.WriteString(w, `[{"id":1,"ref":10,"subject":"Corrigir bug no login","status_extra_info":{"name":"Ready"},"assigned_to_extra_info":{},"project_extra_info":{"name":"App Mobile"}}]`)
		case "/api/v1/issues":
			_, _ = io.WriteString(w, `[{"id":2,"ref":11,"subject":"Melhorar performance","status":5,"project_extra_info":{"name":"Plataforma Web"}}]`)
		case "/api/v1/tasks":
			_, _ = io.WriteString(w, `[]`)
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
	content, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content = %#v, want TextContent", res.Content[0])
	}
	if !strings.Contains(content.Text, "1 resultado") || !strings.Contains(content.Text, "Card") || !strings.Contains(content.Text, "Corrigir bug no login") || !strings.Contains(content.Text, "App Mobile") || !strings.Contains(content.Text, "| Projeto |") {
		t.Errorf("search output inesperado:\n%s", content.Text)
	}

	res, _, err = s.search(context.Background(), nil, SearchInput{Query: "inexistente"})
	if err != nil {
		t.Fatal(err)
	}
	content, _ = res.Content[0].(*mcp.TextContent)
	if !strings.Contains(content.Text, "Nenhum resultado") {
		t.Errorf("search vazio inesperado:\n%s", content.Text)
	}
}


func TestAttachmentEndpoint(t *testing.T) {
	for _, test := range []struct {
		entity   string
		expected string
	}{
		{entity: "userstory", expected: "/userstories"},
		{entity: "card", expected: "/userstories"},
		{entity: "issue", expected: "/issues"},
		{entity: "task", expected: "/tasks"},
		{entity: "subtask", expected: "/tasks"},
	} {
		got, err := attachmentEndpoint(test.entity)
		if err != nil || got != test.expected {
			t.Fatalf("attachmentEndpoint(%q) = %q, %v; want %q", test.entity, got, err, test.expected)
		}
	}
	if _, err := attachmentEndpoint("epic"); err == nil {
		t.Fatal("expected invalid entity error")
	}
}

func TestPaging(t *testing.T) {
	query, paginated, err := paging(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if paginated || len(query) != 0 {
		t.Fatalf("paging without values = %v, %#v; want false and empty query", paginated, query)
	}

	page, pageSize := 2, 50
	query, paginated, err = paging(&page, &pageSize)
	if err != nil {
		t.Fatal(err)
	}
	if !paginated || query.Get("page") != "2" || query.Get("page_size") != "50" {
		t.Fatalf("paging = %v, %#v", paginated, query)
	}
	if _, _, err := paging(nil, intPointer(1001)); err == nil {
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
