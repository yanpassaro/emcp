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
		{name: "palavrao em caixa alta", text: "QUE MERDA", want: "termo impróprio"},
		{name: "palavrao plural", text: "eles são uns idiotas", want: "termo impróprio"},
		{name: "ofensa homofobica", text: "esse viado", want: "termo impróprio"},
		{name: "sigla", text: "vtnc, mano", want: "termo impróprio"},
		{name: "leet numero", text: "que m3rda de deploy", want: "termo impróprio"},
		{name: "leet arroba", text: "seu b@baca", want: "termo impróprio"},
		{name: "letras repetidas", text: "que meeerda", want: "termo impróprio"},
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

func TestCommentAcceptsLegitText(t *testing.T) {
	guard := New(Options{})

	texts := []string{
		"o processo do computador está estável",
		"a classe tem acesso ao sucesso do deploy",
		"precisamos revisar a conta e a contabilidade",
		"o assunto do card é análise de custo",
		"segue o resumo da reunião de alinhamento",
		"o servidor fica no porão do prédio",
	}

	for _, text := range texts {
		if err := guard.Comment(text); err != nil {
			t.Fatalf("falso positivo em %q: %v", text, err)
		}
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
	guard := New(Options{BannedWords: []string{"concorrente"}})

	err := guard.Comment("o concorrente reclamou de novo")
	if err == nil {
		t.Fatal("esperava bloqueio da palavra configurada")
	}
}

func TestAttachmentRejectsBlockedNames(t *testing.T) {
	dir := t.TempDir()
	guard := New(Options{})

	tests := []struct {
		name string
		want string
	}{
		{name: ".env", want: "nome de arquivo bloqueado"},
		{name: ".env.local", want: "nome de arquivo bloqueado"},
		{name: "dump.sql", want: "extensão bloqueada"},
		{name: "id_rsa", want: "nome de arquivo bloqueado"},
		{name: "cert.pem", want: "extensão bloqueada"},
		{name: "prod.kubeconfig", want: "extensão bloqueada"},
		{name: "terraform.tfstate", want: "extensão bloqueada"},
		{name: "chave.asc", want: "extensão bloqueada"},
		{name: "cofre.1pux", want: "extensão bloqueada"},
		{name: "cache.rdb", want: "extensão bloqueada"},
		{name: "captura.har", want: "extensão bloqueada"},
		{name: "sessao.pcap", want: "extensão bloqueada"},
		{name: "backup.zip", want: "compactado ou imagem"},
		{name: "base.tar.gz", want: "compactado ou imagem"},
		{name: "app.apk", want: "compactado ou imagem"},
		{name: "disco.iso", want: "compactado ou imagem"},
		{name: "dump.vmdk", want: "compactado ou imagem"},
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

func TestAttachmentRejectsSensitiveContent(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "config.txt", "senha=SuperSecreta123\n")

	_, _, err := New(Options{}).Attachment(file, "")
	if err == nil {
		t.Fatal("esperava bloqueio por conteúdo")
	}

	if !strings.Contains(err.Error(), "credencial em texto") {
		t.Fatalf("erro = %q", err.Error())
	}
}

func TestAttachmentRejectsSensitiveDescription(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "nota.txt", "combinado por escrito")

	_, _, err := New(Options{}).Attachment(file, "senha=SuperSecreta123")
	if err == nil {
		t.Fatal("esperava bloqueio da descrição")
	}

	if !strings.Contains(err.Error(), "descrição do anexo") {
		t.Fatalf("erro = %q", err.Error())
	}
}

func TestAttachmentAcceptsAnyReadableFile(t *testing.T) {
	dir := t.TempDir()
	guard := New(Options{})

	names := []string{"nota.txt", "foto.png", "planilha.xlsx", "relatorio.pdf", ".rascunho.md"}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			file := writeFile(t, dir, name, "conteudo")

			resolved, size, err := guard.Attachment(file, "resumo do alinhamento")
			if err != nil {
				t.Fatalf("esperava aprovação, veio: %v", err)
			}

			if size <= 0 {
				t.Fatalf("tamanho = %d", size)
			}

			if filepath.Base(resolved) != name {
				t.Fatalf("resolved = %q", resolved)
			}
		})
	}
}

func TestAttachmentRejectsEmptyFile(t *testing.T) {
	dir := t.TempDir()
	file := writeFile(t, dir, "vazio.txt", "")

	_, _, err := New(Options{}).Attachment(file, "")
	if err == nil {
		t.Fatal("esperava bloqueio de arquivo vazio")
	}

	if !strings.Contains(err.Error(), "vazio") {
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
