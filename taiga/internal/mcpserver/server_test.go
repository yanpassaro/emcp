package mcpserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

func TestInfo(t *testing.T) {
	gotMember := ""
	gotPageSize := ""

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v1/users/me":
			writeText(t, w, `{"id":42,"username":"yann","full_name":"Yann Pessoa","email":"«email:4a9e0b4ffbb3»","roles":["Member"],"is_active":true,"date_joined":"2024-03-10T12:00:00Z","last_login":"2026-11-20T09:30:00Z"}`)
		case "/api/v1/projects":
			gotMember = r.URL.Query().Get("member")
			gotPageSize = r.URL.Query().Get("page_size")
			writeText(t, w, `[{"id":7,"name":"Portal","slug":"portal","created_date":"2025-01-08T10:00:00Z","total_activity":123,"i_am_admin":true}]`)
		default:
			t.Errorf("caminho inesperado: %s", r.URL.Path)
		}
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	res, _, err := New(client).get(context.Background(), nil, GetInput{Entity: "info"})
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)

	for _, want := range []string{"# Yann Pessoa", "**ID:** 42", "`yann`", "**Papéis:** Member", "## Projetos que participo (1)", "Portal", "`portal`"} {
		if !strings.Contains(text, want) {
			t.Errorf("output não contém %q:\n%s", want, text)
		}
	}

	if gotMember != "42" {
		t.Errorf("member = %q, want 42", gotMember)
	}

	if gotPageSize != "50" {
		t.Errorf("page_size = %q, want 50", gotPageSize)
	}
}

func TestCommentPosted(t *testing.T) {
	t.Setenv("TAIGA_ALLOW_ACT", "true")

	gotBody := ""

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPatch {
			writeText(t, w, `{"id":123,"ref":5,"subject":"Testar","version":3}`)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}

		gotBody = string(body)
		writeText(t, w, `{"id":123,"ref":5,"subject":"Testar","version":4}`)
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	cardID := 123
	res, _, err := New(client).change(context.Background(), nil, ChangeInput{
		Entity: "card", ItemID: &cardID, Comment: "Segue o ajuste combinado com o time.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(gotBody, `"comment":"Segue o ajuste combinado com o time."`) {
		t.Errorf("PATCH body = %q, sem comment", gotBody)
	}

	if !strings.Contains(gotBody, `"version":3`) {
		t.Errorf("PATCH body = %q, sem version", gotBody)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Comentário publicado") {
		t.Errorf("output não confirma o comentário:\n%s", text)
	}
}

func TestCommentRejected(t *testing.T) {
	t.Setenv("TAIGA_ALLOW_ACT", "true")

	gotMethod := ""

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		gotMethod = r.Method
		writeText(t, w, `{"id":123,"ref":5,"subject":"Testar","version":3}`)
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	cardID := 123
	_, _, err = New(client).change(context.Background(), nil, ChangeInput{
		Entity: "card", ItemID: &cardID, Comment: "a senha=SuperSecreta123 do banco",
	})
	if err == nil {
		t.Fatal("esperava bloqueio do comentário")
	}

	if !strings.Contains(err.Error(), "política de conteúdo") {
		t.Fatalf("erro = %q", err.Error())
	}

	if gotMethod == http.MethodPatch {
		t.Fatal("o PATCH não deveria ter sido enviado")
	}
}

func TestWithoutAllowActRejectsChange(t *testing.T) {
	t.Setenv("TAIGA_ALLOW_ACT", "")

	client, err := taiga.NewClient(taiga.Config{BaseURL: "https://taiga.example.test", Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	cardID := 123
	_, _, err = New(client).change(context.Background(), nil, ChangeInput{
		Entity: "card", ItemID: &cardID, Comment: "segue o ajuste combinado",
	})
	if err == nil {
		t.Fatal("esperava recusa sem TAIGA_ALLOW_ACT")
	}

	if !strings.Contains(err.Error(), "TAIGA_ALLOW_ACT") {
		t.Fatalf("erro = %q", err.Error())
	}
}

func TestWithoutAllowActRejectsUpload(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "nota.txt")
	if err := os.WriteFile(file, []byte("combinado por escrito"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TAIGA_ALLOW_ACT", "")

	client, err := taiga.NewClient(taiga.Config{BaseURL: "https://taiga.example.test", Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = New(client).attachment(context.Background(), nil, AttachmentInput{
		Entity: "card", Action: "upload", ObjectID: 1, ProjectID: 2, SourcePath: file,
	})
	if err == nil {
		t.Fatal("esperava recusa sem TAIGA_ALLOW_ACT")
	}

	if !strings.Contains(err.Error(), "TAIGA_ALLOW_ACT") {
		t.Fatalf("erro = %q", err.Error())
	}
}

func TestAllowActControlsDescriptions(t *testing.T) {
	t.Setenv("TAIGA_ALLOW_ACT", "1")

	client, err := taiga.NewClient(taiga.Config{BaseURL: "https://taiga.example.test", Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	New(client).Register(server)

	if got := attachmentDescription(false); strings.Contains(got, "upload") {
		t.Fatalf("descrição sem escrita menciona upload: %q", got)
	}

	if got := attachmentDescription(true); !strings.Contains(got, "upload") {
		t.Fatalf("descrição com escrita sem upload: %q", got)
	}
}

func TestUploadRejectedDangerousName(t *testing.T) {
	t.Setenv("TAIGA_ALLOW_ACT", "true")

	dir := t.TempDir()
	file := filepath.Join(dir, "dump.dmp")
	if err := os.WriteFile(file, []byte("combinado por escrito"), 0o600); err != nil {
		t.Fatal(err)
	}

	client, err := taiga.NewClient(taiga.Config{BaseURL: "https://taiga.example.test", Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = New(client).attachment(context.Background(), nil, AttachmentInput{
		Entity: "card", Action: "upload", ObjectID: 1, ProjectID: 2, SourcePath: file,
	})
	if err == nil {
		t.Fatal("esperava recusa do upload")
	}

	if !strings.Contains(err.Error(), "extensão bloqueada") {
		t.Fatalf("erro = %q", err.Error())
	}
}

func TestUploadAccepted(t *testing.T) {
	t.Setenv("TAIGA_ALLOW_ACT", "true")

	dir := t.TempDir()
	file := filepath.Join(dir, "nota.txt")
	if err := os.WriteFile(file, []byte("combinado por escrito"), 0o600); err != nil {
		t.Fatal(err)
	}

	gotPath := ""
	gotBody := ""

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}

		gotPath = r.URL.Path
		gotBody = string(body)
		writeText(t, w, `{"id":9,"name":"nota.txt","url":"/media/nota.txt?token=abc#_taiga-refresh=userstory:9"}`)
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	res, _, err := New(client).attachment(context.Background(), nil, AttachmentInput{
		Entity: "card", Action: "upload", ObjectID: 1, ProjectID: 2, SourcePath: file, Description: "resumo do alinhamento", FromComment: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotPath != "/api/v1/userstories/attachments" {
		t.Errorf("path = %q", gotPath)
	}

	for _, want := range []string{`name="object_id"`, `name="project"`, `filename="nota.txt"`, "combinado por escrito"} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("multipart sem %q:\n%s", want, gotBody)
		}
	}

	if !strings.Contains(resultText(t, res), "Anexo enviado") {
		t.Errorf("output sem confirmação")
	}

	if strings.Contains(gotBody, `name="project_id"`) {
		t.Errorf("multipart enviou project_id; o Taiga espera o campo 'project':\n%s", gotBody)
	}

	if !strings.Contains(gotBody, `name="from_comment"`) {
		t.Errorf("multipart sem from_comment:\n%s", gotBody)
	}

	output := resultText(t, res)
	if !strings.Contains(output, "![nota.txt](/media/nota.txt?token=abc#_taiga-refresh=userstory:9)") {
		t.Errorf("output sem markdown pronto:\n%s", output)
	}
}

func TestStripAttachmentRefs(t *testing.T) {
	ref := "![evidencia.png](http://192.168.0.10/media/e.png?token=abc#_taiga-refresh=userstory:9)"

	if got := stripAttachmentRefs(ref); got != "[anexo]" {
		t.Fatalf("stripAttachmentRefs = %q", got)
	}

	if got := stripAttachmentRefs("texto com 192.168.0.10 solto"); !strings.Contains(got, "192.168.0.10") {
		t.Fatalf("o strip não deve mascarar IP fora de referência de anexo: %q", got)
	}
}

func TestCommentsHideDeleted(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v1/userstories/123":
			writeText(t, w, `{"id":123,"ref":5,"subject":"Testar","project":7,"version":3,"tags":[]}`)
		case "/api/v1/history/userstory/123":
			writeText(t, w, `[
				{"comment":"Comentario vivo","created_at":"2026-01-01T10:00:00Z","user":{"name":"Ana"}},
				{"comment":"Comentario apagado","created_at":"2026-01-02T10:00:00Z","user":{"name":"Ana"},"delete_comment_date":"2026-01-03T10:00:00Z"},
				{"comment":"Comentario oculto","created_at":"2026-01-02T11:00:00Z","user":{"name":"Ana"},"is_hidden":true}
			]`)
		default:
			writeText(t, w, `[]`)
		}
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	cardID := 123
	res, _, err := New(client).get(context.Background(), nil, GetInput{Entity: "card", ID: &cardID})
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)

	if !strings.Contains(text, "Comentario vivo") {
		t.Errorf("output sem o comentário ativo:\n%s", text)
	}

	for _, hidden := range []string{"Comentario apagado", "Comentario oculto"} {
		if strings.Contains(text, hidden) {
			t.Errorf("output exibiu %q:\n%s", hidden, text)
		}
	}
}

func TestSearchScopes(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v1/projects":
			writeText(t, w, `[{"id":7,"name":"Portal do Cliente","slug":"portal"}]`)
		case "/api/v1/search":
			writeText(t, w, `{"wikipages":[{"id":3,"slug":"onboarding-portal"}],"count":1}`)
		default:
			writeText(t, w, `[]`)
		}
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	projectID := 7
	res, _, err := New(client).search(context.Background(), nil, SearchInput{
		ProjectID: &projectID, Query: "portal", Scope: "projects,wiki",
	})
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	for _, want := range []string{"Projeto", "Portal do Cliente", "Wiki", "onboarding-portal"} {
		if !strings.Contains(text, want) {
			t.Errorf("output sem %q:\n%s", want, text)
		}
	}
}

func TestSearchComments(t *testing.T) {
	gotQuery := ""

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path != "/api/v1/history/userstory/55" {
			t.Errorf("caminho inesperado: %s", r.URL.Path)
			return
		}

		gotQuery = r.URL.Query().Get("type")
		writeText(t, w, `[
			{"id":11,"comment":"cliente pediu ajuste no portal","created_at":"2026-01-01T10:00:00Z","user":{"name":"Ana"}},
			{"id":12,"comment":"nada a ver","created_at":"2026-01-02T10:00:00Z","user":{"name":"Ana"}},
			{"id":13,"comment":"portal apagado","created_at":"2026-01-03T10:00:00Z","user":{"name":"Ana"},"delete_comment_date":"2026-01-04T10:00:00Z"}
		]`)
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	cardID := 55
	res, _, err := New(client).search(context.Background(), nil, SearchInput{
		Query: "portal", Scope: "comments", CardID: &cardID,
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotQuery != "comment" {
		t.Errorf("type = %q, esperava comment", gotQuery)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "cliente pediu ajuste no portal") {
		t.Errorf("output sem o comentário:\n%s", text)
	}

	if strings.Contains(text, "portal apagado") {
		t.Errorf("output exibiu comentário apagado:\n%s", text)
	}
}

func TestDeleteAttachment(t *testing.T) {
	t.Setenv("TAIGA_ALLOW_ACT", "true")

	gotMethod := ""
	gotPath := ""

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		gotMethod, gotPath = r.Method, r.URL.Path
		writeText(t, w, `{}`)
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	res, _, err := New(client).attachment(context.Background(), nil, AttachmentInput{
		Entity: "card", Action: "delete", AttachmentID: 9,
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q", gotMethod)
	}

	if gotPath != "/api/v1/userstories/attachments/9" {
		t.Errorf("path = %q", gotPath)
	}

	if !strings.Contains(resultText(t, res), "removido") {
		t.Errorf("output sem confirmação")
	}
}

func TestDeleteComment(t *testing.T) {
	t.Setenv("TAIGA_ALLOW_ACT", "true")

	gotPath := ""
	gotID := ""

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v1/userstories/123":
			writeText(t, w, `{"id":123,"ref":5,"subject":"Testar","version":3}`)
		default:
			gotPath = r.URL.Path
			gotID = r.URL.Query().Get("id")
			writeText(t, w, `{}`)
		}
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	cardID := 123
	commentID := 77
	res, _, err := New(client).change(context.Background(), nil, ChangeInput{
		Entity: "card", ItemID: &cardID, Action: "delete_comment", CommentID: &commentID,
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotPath != "/api/v1/history/userstory/123/delete_comment" {
		t.Errorf("path = %q", gotPath)
	}

	if gotID != "77" {
		t.Errorf("id = %q", gotID)
	}

	if !strings.Contains(resultText(t, res), "apagado") {
		t.Errorf("output sem confirmação")
	}
}

func TestActivityDateRange(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v1/userstories":
			writeText(t, w, `[
				{"id":1,"ref":1,"subject":"Antigo","modified_date":"2026-01-01T10:00:00Z","status_extra_info":{"name":"New"}},
				{"id":2,"ref":2,"subject":"Recente","modified_date":"2026-01-20T10:00:00Z","status_extra_info":{"name":"New"}}
			]`)
		default:
			writeText(t, w, `[]`)
		}
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	res, _, err := New(client).listActivity(context.Background(), ListActivityInput{
		Since: "2026-01-15",
	})
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Recente") {
		t.Errorf("output sem o item dentro do range:\n%s", text)
	}

	if strings.Contains(text, "Antigo") {
		t.Errorf("output exibiu item fora do range:\n%s", text)
	}
}

func TestChangeStatusCard(t *testing.T) {
	t.Setenv("TAIGA_ALLOW_ACT", "true")

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
	res, _, err := s.change(context.Background(), nil, ChangeInput{
		Entity: "card", ItemID: &cardID, StatusID: intPointer(7),
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
	t.Setenv("TAIGA_ALLOW_ACT", "true")

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
	res, _, err := s.change(context.Background(), nil, ChangeInput{
		Entity: "issue", ItemID: &issueID, StatusID: intPointer(3),
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

func TestChangeTagsCard(t *testing.T) {
	t.Setenv("TAIGA_ALLOW_ACT", "true")

	gotMethod := ""
	gotPath := ""
	gotBody := ""

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPatch {
			writeText(t, w, `{"id":123,"ref":5,"subject":"Testar","status_extra_info":{"name":"In Progress"},"tags":["bug",["urgente","#ff0000"]],"version":3}`)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}

		gotMethod, gotPath, gotBody = r.Method, r.URL.Path, string(body)
		writeText(t, w, `{"id":123,"ref":5,"subject":"Testar","status_extra_info":{"name":"In Progress"},"tags":["bug","antiga"],"version":4}`)
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	s := New(client)
	cardID := 123
	res, _, err := s.change(context.Background(), nil, ChangeInput{
		Entity: "card", ItemID: &cardID, AddTags: []string{"antiga"}, RemoveTags: []string{"Urgente"},
	})
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)

	for _, want := range []string{"✅ Card #5", "Tags adicionadas:** antiga", "Tags removidas:** Urgente", "Tags atuais:** bug, antiga"} {
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

	if strings.TrimSpace(gotBody) != `{"tags":["bug","antiga"],"version":3}` {
		t.Errorf("PATCH body = %q", gotBody)
	}
}

func TestChangeAssignees(t *testing.T) {
	t.Setenv("TAIGA_ALLOW_ACT", "true")

	gotMethod := ""
	gotPath := ""
	gotBody := ""

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPatch {
			writeText(t, w, `{"id":123,"ref":5,"subject":"Testar","status_extra_info":{"name":"In Progress"},"assigned_to":1,"assigned_to_extra_info":{"id":1,"full_name_display":"Ana"},"assigned_users":[1,2],"version":3}`)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}

		gotMethod, gotPath, gotBody = r.Method, r.URL.Path, string(body)
		writeText(t, w, `{"id":123,"ref":5,"subject":"Testar","status_extra_info":{"name":"In Progress"},"assigned_to":null,"assigned_users":[2,3],"version":4}`)
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	s := New(client)
	cardID := 123
	res, _, err := s.change(context.Background(), nil, ChangeInput{
		Entity: "card", ItemID: &cardID, AssignedUsers: []int{3, 2},
	})
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)

	for _, want := range []string{"✅ Card #5", "Responsáveis:** 2, 3"} {
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

	if strings.TrimSpace(gotBody) != `{"assigned_to":null,"assigned_users":[3,2],"version":3}` {
		t.Errorf("PATCH body = %q", gotBody)
	}
}

func TestChangeAssignedTo(t *testing.T) {
	t.Setenv("TAIGA_ALLOW_ACT", "true")

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

		gotBody = string(body)
		writeText(t, w, `{"id":77,"ref":9,"subject":"Bug no login","status_extra_info":{"name":"New"},"assigned_to":4,"assigned_to_extra_info":{"id":4,"full_name_display":"Bruno"},"assigned_users":[4],"version":2}`)
	}))
	defer backend.Close()

	client, err := taiga.NewClient(taiga.Config{BaseURL: backend.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	s := New(client)
	issueID := 77
	res, _, err := s.change(context.Background(), nil, ChangeInput{
		Entity: "issue", ItemID: &issueID, AssignedTo: intPointer(4),
	})
	if err != nil {
		t.Fatal(err)
	}

	text := resultText(t, res)
	if !strings.Contains(text, "Responsável:** Bruno (4)") {
		t.Errorf("output não contém responsável:\n%s", text)
	}

	if strings.TrimSpace(gotBody) != `{"assigned_to":4,"version":1}` {
		t.Errorf("PATCH body = %q", gotBody)
	}
}

func TestChangeRequiresTarget(t *testing.T) {
	t.Setenv("TAIGA_ALLOW_ACT", "true")

	client, err := taiga.NewClient(taiga.Config{BaseURL: "https://taiga.example.test", Token: "token"})
	if err != nil {
		t.Fatal(err)
	}

	s := New(client)
	cardID := 123
	_, _, err = s.change(context.Background(), nil, ChangeInput{
		Entity: "card", ItemID: &cardID,
	})
	if err == nil {
		t.Fatal("expected validation error")
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
