package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommentBlocksContent(t *testing.T) {
	guard := New(Options{})

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "limpo", text: "Segue o ajuste combinado com o time.", want: ""},
		{name: "senha", text: "usa senha=SuperSecreta123 no banco", want: "credencial em texto"},
		{name: "token gitlab", text: "pega o glpat-abcdefghij1234567890", want: "token do GitLab"},
		{name: "email", text: fmt.Sprintf("manda para %s@%s", "contato", "exemplo.com"), want: "e-mail"},
		{name: "cpf", text: fmt.Sprintf("documento %s.%s.%s-%s", "123", "456", "789", "01"), want: "CPF"},
		{name: "ip interno", text: "o servidor é 192.168.0.10", want: "IP interno"},
		{name: "hostname interno", text: "chama o banco.prod.internal", want: "hostname interno"},
		{name: "palavrao", text: "essa merda de deploy", want: "termo impróprio"},
		{name: "palavrao com acento", text: "que otário", want: "termo impróprio"},
		{name: "palavrao sem acento", text: "que otario", want: "termo impróprio"},
		{name: "palavrao plural", text: "eles são uns idiotas", want: "termo impróprio"},
		{name: "ofensa homofobica", text: "esse viado", want: "termo impróprio"},
		{name: "sigla", text: "vtnc, mano", want: "termo impróprio"},
		{name: "chave privada", text: "-----BEGIN RSA PRIVATE KEY-----", want: "chave privada"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := guard.Comment(test.text)
			if test.want == "" {
				if err != nil {
					t.Fatalf("esperava aprovação, veio: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("esperava bloqueio por %q", test.want)
			}

			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("erro = %q, esperava %q", err.Error(), test.want)
			}
		})
	}
}

func TestCommentBlocksLongText(t *testing.T) {
	guard := New(Options{})
	text := strings.Repeat("a", MAX_COMMENT_CHARS+1)

	if err := guard.Comment(text); err == nil {
		t.Fatal("esperava bloqueio por tamanho")
	}
}

func TestCommentAcceptsCustomBannedWords(t *testing.T) {
	guard := New(Options{BannedWords: []string{"cliente-chato"}})

	err := guard.Comment("o cliente-chato reclamou de novo")
	if err == nil {
		t.Fatal("esperava bloqueio da palavra configurada")
	}
}

func TestCommentLoadsWordsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "palavras.txt")
	content := "# termos internos\nconcorrente-x, processo-antigo\n\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	guard := New(Options{BannedWordsFile: path})

	for _, text := range []string{"falar mal do concorrente-x", "voltar com o processo-antigo"} {
		if err := guard.Comment(text); err == nil {
			t.Fatalf("esperava bloqueio de %q", text)
		}
	}

	if err := guard.Comment("segue o ajuste combinado"); err != nil {
		t.Fatalf("esperava aprovação, veio: %v", err)
	}
}

func TestCommentFailsClosedWithoutWordsFile(t *testing.T) {
	guard := New(Options{BannedWordsFile: filepath.Join(t.TempDir(), "nao-existe.txt")})

	err := guard.Comment("segue o ajuste combinado")
	if err == nil {
		t.Fatal("esperava falha fechada")
	}

	if !strings.Contains(err.Error(), "indisponível") {
		t.Fatalf("erro = %q", err.Error())
	}
}

func TestAttachmentRequiresUploadDirs(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "nota.txt", "combinado por escrito")

	_, _, err := New(Options{}).Attachment(file, "")
	if err == nil {
		t.Fatal("esperava upload desativado")
	}

	if !strings.Contains(err.Error(), "TAIGA_UPLOAD_DIRS") {
		t.Fatalf("erro = %q", err.Error())
	}
}

func TestAttachmentRejectsOutsideDirs(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	file := writeFile(t, outside, "nota.txt", "combinado por escrito")

	_, _, err := New(Options{UploadDirs: []string{allowed}}).Attachment(file, "")
	if err == nil {
		t.Fatal("esperava bloqueio fora dos diretórios")
	}

	if !strings.Contains(err.Error(), "TAIGA_UPLOAD_DIRS") {
		t.Fatalf("erro = %q", err.Error())
	}
}

func TestAttachmentRejectsBlockedNames(t *testing.T) {
	dir := t.TempDir()
	guard := New(Options{UploadDirs: []string{dir}})

	tests := []struct {
		name string
		want string
	}{
		{name: ".env", want: "oculto"},
		{name: "dump.sql", want: "extensão bloqueada"},
		{name: "id_rsa", want: "nome de arquivo bloqueado"},
		{name: "backup.zip", want: "arquivos compactados"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := writeFile(t, dir, test.name, "conteudo")

			_, _, err := guard.Attachment(file, "")
			if err == nil {
				t.Fatalf("esperava bloqueio de %s", test.name)
			}

			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("erro = %q, esperava %q", err.Error(), test.want)
			}
		})
	}
}

func TestAttachmentAllowsArchivesWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "backup.zip", "conteudo")

	_, _, err := New(Options{UploadDirs: []string{dir}, AllowArchives: true}).Attachment(file, "")
	if err != nil {
		t.Fatalf("esperava aprovação, veio: %v", err)
	}
}

func TestAttachmentRejectsSensitiveContent(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "config.txt", "senha=SuperSecreta123\n")

	_, _, err := New(Options{UploadDirs: []string{dir}}).Attachment(file, "")
	if err == nil {
		t.Fatal("esperava bloqueio por conteúdo")
	}

	if !strings.Contains(err.Error(), "credencial em texto") {
		t.Fatalf("erro = %q", err.Error())
	}
}

func TestAttachmentRejectsHiddenFiles(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, ".rascunho.txt", "combinado")

	_, _, err := New(Options{UploadDirs: []string{dir}}).Attachment(file, "")
	if err == nil {
		t.Fatal("esperava bloqueio de arquivo oculto")
	}

	if !strings.Contains(err.Error(), "oculto") {
		t.Fatalf("erro = %q", err.Error())
	}
}

func TestAttachmentAcceptsRegularFile(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "nota.txt", "combinado por escrito")

	resolved, size, err := New(Options{UploadDirs: []string{dir}}).Attachment(file, "resumo do alinhamento")
	if err != nil {
		t.Fatalf("esperava aprovação, veio: %v", err)
	}

	if size <= 0 {
		t.Fatalf("tamanho = %d", size)
	}

	if filepath.Base(resolved) != "nota.txt" {
		t.Fatalf("resolved = %q", resolved)
	}
}

func TestAttachmentRejectsSensitiveDescription(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "nota.txt", "combinado por escrito")

	_, _, err := New(Options{UploadDirs: []string{dir}}).Attachment(file, "senha=SuperSecreta123")
	if err == nil {
		t.Fatal("esperava bloqueio da descrição")
	}

	if !strings.Contains(err.Error(), "descrição do anexo") {
		t.Fatalf("erro = %q", err.Error())
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
