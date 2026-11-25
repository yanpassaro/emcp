package policy

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

const (
	MAX_COMMENT_CHARS        = 4000
	MAX_SAMPLE_BYTES         = 64 * 1024
	DEFAULT_MAX_UPLOAD_BYTES = 10 * 1024 * 1024
)

var defaultBannedWords = []string{
	"idiota", "idiotas", "idiotice", "imbecil", "imbecis", "babaca", "babacas",
	"babaquice", "babacao", "otario", "otaria", "otarios", "otarias", "otarice",
	"panaca", "panacas", "trouxa", "trouxas", "burro", "burra", "burros", "burras",
	"anta", "antas", "jumento", "jumenta", "paspalho", "paspalha", "estupido",
	"estupida", "estupidos", "estupidas", "cretino", "cretina", "cretinos",
	"cretinas", "pateta", "patetas", "retardado", "retardada", "retardados",
	"retardadas", "mongol", "mongoloide", "mongoloides", "debiloide", "escroto",
	"escrota", "escrotos", "escrotas", "nojento", "nojenta", "nojentas", "imundo",
	"imunda", "canalha", "canalhas", "safado", "safada", "safados", "safadas",
	"pilantra", "pilantras", "picareta", "picaretas", "vigarista", "vigaristas",
	"mentiroso", "mentirosa", "covarde", "covardes", "palhaco", "palhaca", "palhacos",
	"merda", "merdas", "merdinha", "merdice", "merdao", "bosta", "bostas",
	"bostinha", "cagada", "cagadas", "cagar", "caguei", "porra", "porras",
	"porrinha", "caralho", "caralhos", "caralhada", "cacete", "cacetes",
	"caceta", "foda", "foder", "fodeu", "fudido", "fudida", "krl", "kct", "kceta",
	"pqp", "pnc", "tnc", "vtnc", "fdp", "vsf", "vsfd", "arrombado", "arrombada",
	"arrombados", "arrombadas", "puta", "putas", "puto", "putos", "putaria",
	"putarias", "putona", "putonas", "putaco", "puteiro", "puteiros", "piranha",
	"piranhas", "vagabunda", "vagabundas", "vagaba", "vagabundo", "vadia", "vadias",
	"biscate", "biscates", "quenga", "quengas", "rameira", "rameiras", "corno",
	"cornos", "cornuda", "cornudo", "cornudas", "cornudos", "cu", "cus", "cuzao",
	"cuzinho", "buceta", "bucetas", "boceta", "bocetas", "xoxota", "xoxotas",
	"xereca", "xerecas", "boquete", "boquetes", "punheta", "punhetas", "siririca",
	"tesao", "tesudo", "tesuda", "viado", "viada", "viados", "viadao", "baitola",
	"baitolas", "bicha", "bichas", "bichinha", "boiola", "boiolas", "biba", "sapatao",
	"sapatona", "sapatonas", "traveco", "travecas", "crioulo", "crioula", "crioulos",
	"crioulas", "macumbeiro", "macumbeira", "favelado", "favelada", "favelados",
	"faveladas", "aleijado", "aleijada", "desgracado", "desgracada", "desgracados",
	"desgracadas", "fuck", "fucking", "fucker", "shit", "shitty", "bitch", "bitches",
	"asshole", "assholes", "bastard", "bastards", "dumbass", "idiot", "idiots",
	"moron", "morons", "retard", "retards", "cunt", "dick", "dickhead", "prick",
	"whore", "slut", "fag", "faggot", "motherfucker",
}

var accentFolder = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

var leetFolder = strings.NewReplacer(
	"0", "o", "3", "e", "4", "a", "@", "a",
)

var secretPatterns = []struct {
	label   string
	pattern *regexp.Regexp
}{
	{"chave privada", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"token do GitLab", regexp.MustCompile(`glpat-[A-Za-z0-9_\-]{20,}`)},
	{"token do GitHub", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`)},
	{"chave da AWS", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"token do Slack", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9\-]{10,}`)},
	{"chave de API", regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}`)},
	{"credencial em texto", regexp.MustCompile(`(?i)\b(senha|password|passwd|secret|token|api[_-]?key|apikey)\b\s*[:=]\s*\S{4,}`)},
	{"URL com credencial", regexp.MustCompile(`(?i)\b(postgres|postgresql|mysql|mariadb|mongodb|redis|amqp)://[^\s:@/]+:[^\s@/]+@`)},
	{"e-mail", regexp.MustCompile(`(?i)[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)},
	{"CPF", regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`)},
	{"CNPJ", regexp.MustCompile(`\b\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}\b`)},
	{"telefone", regexp.MustCompile(`\(?\b\d{2}\)?[\s-]\d{4,5}[\s-]?\d{4}\b`)},
	{"IP interno", regexp.MustCompile(`\b(?:10\.\d{1,3}|192\.168|172\.(?:1[6-9]|2\d|3[01]))\.\d{1,3}\.\d{1,3}\b`)},
	{"hostname interno", regexp.MustCompile(`(?i)\b[a-z0-9][a-z0-9\-]*\.(internal|local|lan|corp|intranet|localdomain)\b`)},
}

var blockedExtensions = []string{
	".env", ".envrc", ".npmrc", ".netrc", ".pgpass",
	".pem", ".key", ".crt", ".cert", ".cer", ".der", ".csr", ".p7b", ".p7c",
	".p8", ".pfx", ".p12", ".jks", ".keystore", ".bks", ".ppk",
	".gpg", ".pgp", ".asc",
	".kdb", ".kdbx", ".psafe3", ".1pux", ".1pif",
	".ovpn", ".kubeconfig",
	".tfstate", ".tfvars",
	".sql", ".dump", ".dmp", ".bak", ".sqlite", ".sqlite3", ".db", ".db3",
	".mdb", ".accdb", ".dbf", ".mdf", ".ldf", ".frm", ".ibd", ".myd", ".myi",
	".bson", ".rdb", ".aof", ".ldb", ".sst",
	".har", ".pcap", ".pcapng", ".ldif",
}

var blockedNames = []string{
	"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519", "credentials",
	"secrets", "secret", "kubeconfig", ".npmrc", ".netrc", ".pgpass",
}

var blockedArchives = []string{
	".zip", ".zipx", ".z01", ".rar", ".r00", ".7z", ".tar", ".gz", ".tgz",
	".bz2", ".tbz", ".tbz2", ".xz", ".txz", ".lz", ".lz4", ".lzma",
	".zst", ".zstd", ".br", ".z", ".ace", ".arj", ".lzh", ".lha", ".cab",
	".sit", ".sitx", ".cpio", ".iso", ".img", ".dmg", ".vhd", ".vhdx",
	".vmdk", ".ova", ".jar", ".war", ".ear", ".apk", ".aab", ".ipa",
	".deb", ".rpm", ".msi", ".msp", ".pkg", ".appimage", ".snap",
}

type Options struct {
	BannedWords    []string
	MaxUploadBytes int64
}

type Guard struct {
	bannedWords    []string
	maxUploadBytes int64
}

func New(opts Options) *Guard {
	maxUploadBytes := opts.MaxUploadBytes
	if maxUploadBytes <= 0 {
		maxUploadBytes = DEFAULT_MAX_UPLOAD_BYTES
	}

	return &Guard{
		bannedWords:    normalizeWords(append(slices.Clone(defaultBannedWords), opts.BannedWords...)),
		maxUploadBytes: maxUploadBytes,
	}
}

func (g *Guard) Comment(text string) error {
	value := strings.TrimSpace(text)
	if value == "" {
		return errors.New("texto vazio")
	}

	if len([]rune(value)) > MAX_COMMENT_CHARS {
		return fmt.Errorf("texto com mais de %d caracteres", MAX_COMMENT_CHARS)
	}

	reasons := secretReasons(value)
	banned := bannedReason(g.bannedWords, value)
	if banned != "" {
		reasons = append(reasons, banned)
	}

	if len(reasons) == 0 {
		return nil
	}

	return fmt.Errorf("bloqueado pela política de conteúdo: %s", strings.Join(reasons, "; "))
}

func (g *Guard) Attachment(path, description string) (string, int64, error) {
	description = strings.TrimSpace(description)
	if description != "" {
		if err := g.Comment(description); err != nil {
			return "", 0, fmt.Errorf("descrição do anexo: %w", err)
		}
	}

	resolved, size, err := g.uploadTarget(path)
	if err != nil {
		return "", 0, err
	}

	reasons, err := scanFile(resolved)
	if err != nil {
		return "", 0, err
	}

	if len(reasons) == 0 {
		return resolved, size, nil
	}

	return "", 0, fmt.Errorf("arquivo bloqueado pela política de conteúdo: %s", strings.Join(reasons, "; "))
}

func (g *Guard) uploadTarget(path string) (string, int64, error) {
	value := strings.TrimSpace(path)
	if value == "" {
		return "", 0, errors.New("informe source_path com o arquivo a enviar")
	}

	abs, err := filepath.Abs(value)
	if err != nil {
		return "", 0, fmt.Errorf("caminho inválido: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", 0, fmt.Errorf("não foi possível abrir o arquivo: %w", err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", 0, fmt.Errorf("não foi possível ler o arquivo: %w", err)
	}

	if !info.Mode().IsRegular() {
		return "", 0, errors.New("apenas arquivos regulares podem ser enviados")
	}

	if info.Size() == 0 {
		return "", 0, errors.New("arquivo vazio")
	}

	if info.Size() > g.maxUploadBytes {
		return "", 0, fmt.Errorf("arquivo maior que o limite de %d bytes", g.maxUploadBytes)
	}

	if err := checkName(filepath.Base(resolved)); err != nil {
		return "", 0, err
	}

	return resolved, info.Size(), nil
}

func scanFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("não foi possível abrir o arquivo: %w", err)
	}
	defer file.Close()

	sample := make([]byte, MAX_SAMPLE_BYTES)
	n, err := io.ReadFull(file, sample)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, fmt.Errorf("não foi possível ler o arquivo: %w", err)
			}
		}
	}

	return secretReasons(string(sample[:n])), nil
}

func secretReasons(text string) []string {
	reasons := []string{}
	for _, rule := range secretPatterns {
		if rule.pattern.MatchString(text) {
			reasons = append(reasons, fmt.Sprintf("%s detectado", rule.label))
		}
	}
	return reasons
}

func bannedReason(banned []string, text string) string {
	for _, token := range tokens(text) {
		if slices.Contains(banned, token) {
			return "termo impróprio detectado"
		}
	}
	return ""
}

func tokens(text string) []string {
	folded := foldText(text)
	parts := strings.FieldsFunc(folded, func(r rune) bool { return !unicode.IsLetter(r) })
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, part)
	}
	return out
}

func foldText(text string) string {
	value := accentFolder.Replace(strings.ToLower(text))
	value = leetFolder.Replace(value)
	return collapseRepeats(value)
}

func collapseRepeats(value string) string {
	b := strings.Builder{}
	previous := rune(0)
	for _, r := range value {
		if r == previous {
			continue
		}
		b.WriteRune(r)
		previous = r
	}
	return b.String()
}

func normalizeWords(words []string) []string {
	out := make([]string, 0, len(words))
	for _, word := range words {
		value := strings.TrimSpace(foldText(word))
		if value == "" {
			continue
		}
		if slices.Contains(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func checkName(name string) error {
	lower := strings.ToLower(name)

	if slices.Contains(blockedNames, lower) {
		return fmt.Errorf("nome de arquivo bloqueado: %s", name)
	}

	if strings.HasPrefix(lower, ".env") {
		return fmt.Errorf("nome de arquivo bloqueado: %s", name)
	}

	ext := filepath.Ext(lower)

	if slices.Contains(blockedExtensions, ext) {
		return fmt.Errorf("extensão bloqueada: %s", name)
	}

	if slices.Contains(blockedArchives, ext) {
		return fmt.Errorf("tipo de arquivo bloqueado (compactado ou imagem): %s", name)
	}

	return nil
}
