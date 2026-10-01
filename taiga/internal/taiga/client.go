package taiga

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const (
	DEFAULT_BASE_URL          = "https://api.taiga.io"
	DEFAULT_MAX_DOWNLOAD      = 100 * 1024 * 1024
	MAX_JSON_RESPONSE_BYTES   = 32 * 1024 * 1024
	MAX_ERROR_RESPONSE_BYTES  = 8 * 1024
	DEFAULT_AUTH_SCHEME       = "Bearer"
	DEFAULT_TIMEOUT           = 60 * time.Second
	MAX_DOWNLOAD_ATTEMPTS     = 10000
	DEFAULT_ATTACHMENT_NAME   = "attachment"
	DISABLE_PAGINATION_HEADER = "x-disable-pagination"
	HTTP_STATUS_RANGE_MIN     = 200
	HTTP_STATUS_RANGE_MAX     = 300
)

type Config struct {
	BaseURL          string
	Token            string
	AuthScheme       string
	HTTPClient       *http.Client
	DownloadDir      string
	MaxDownloadBytes int64
}

type Client struct {
	baseURL          *url.URL
	token            string
	authScheme       string
	httpClient       *http.Client
	downloadDir      string
	maxDownloadBytes int64
}

type ResponseMeta struct {
	Paginated    string `json:"paginated,omitempty"`
	PaginatedBy  string `json:"paginated_by,omitempty"`
	Total        string `json:"total,omitempty"`
	CurrentPage  string `json:"current_page,omitempty"`
	NextPage     string `json:"next_page,omitempty"`
	PreviousPage string `json:"previous_page,omitempty"`
}

type DownloadResult struct {
	Path        string `json:"path"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type,omitempty"`
	SourceURL   string `json:"source_url"`
}

func NewClient(cfg Config) (*Client, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		base = DEFAULT_BASE_URL
	}

	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid TAIGA_URL: %w", err)
	}

	invalid := false
	if u.Scheme != "http" {
		if u.Scheme != "https" {
			invalid = true
		}
	}

	if invalid {
		return nil, errors.New("TAIGA_URL must use http or https")
	}

	if u.Host == "" {
		return nil, errors.New("TAIGA_URL must include a host")
	}

	if u.RawQuery != "" {
		return nil, errors.New("TAIGA_URL must not contain a query string or fragment")
	}

	if u.Fragment != "" {
		return nil, errors.New("TAIGA_URL must not contain a query string or fragment")
	}

	basePath := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(basePath, "/api/v1") {
		basePath += "/api/v1"
	}

	u.Path = basePath
	u.RawPath = ""

	token := strings.TrimSpace(cfg.Token)
	authScheme := strings.TrimSpace(cfg.AuthScheme)
	if authScheme == "" {
		authScheme = DEFAULT_AUTH_SCHEME
	}

	if strings.ContainsAny(authScheme, " \t\r\n") {
		return nil, errors.New("TAIGA_AUTH_SCHEME must not contain whitespace")
	}

	downloadDir := strings.TrimSpace(cfg.DownloadDir)
	if downloadDir == "" {
		downloadDir = filepath.Join(UserLocalDir(), "mcp", "taiga", "downloads")
	}

	absDir, err := filepath.Abs(downloadDir)
	if err != nil {
		return nil, fmt.Errorf("invalid download directory: %w", err)
	}

	maxDownloadBytes := cfg.MaxDownloadBytes
	if maxDownloadBytes <= 0 {
		maxDownloadBytes = DEFAULT_MAX_DOWNLOAD
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DEFAULT_TIMEOUT}
	}

	return &Client{
		baseURL:          u,
		token:            token,
		authScheme:       authScheme,
		httpClient:       httpClient,
		downloadDir:      absDir,
		maxDownloadBytes: maxDownloadBytes,
	}, nil
}

func UserLocalDir() string {
	home := os.Getenv("USERPROFILE")
	if home != "" {
		return filepath.Join(home, ".local", "share")
	}

	home = os.Getenv("HOME")
	if home != "" {
		return filepath.Join(home, ".local", "share")
	}

	return filepath.Join(".", "taiga-mcp")
}

func isSuccess(status int) bool {
	if status < HTTP_STATUS_RANGE_MIN {
		return false
	}
	return status < HTTP_STATUS_RANGE_MAX
}

func errorMessage(resp *http.Response) string {
	detail, err := io.ReadAll(io.LimitReader(resp.Body, MAX_ERROR_RESPONSE_BYTES))
	if err != nil {
		return http.StatusText(resp.StatusCode)
	}

	message := strings.TrimSpace(string(detail))
	if message == "" {
		return http.StatusText(resp.StatusCode)
	}

	return message
}

func (c *Client) postAuth(ctx context.Context, endpoint string, payload map[string]string) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal auth payload: %w", err)
	}

	u := c.endpointURL(endpoint, nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth request: %w", err)
	}
	defer resp.Body.Close()

	if !isSuccess(resp.StatusCode) {
		return "", fmt.Errorf("auth failed (HTTP %d): %s", resp.StatusCode, errorMessage(resp))
	}

	result := struct {
		AuthToken string `json:"auth_token"`
	}{}

	if err := json.NewDecoder(io.LimitReader(resp.Body, MAX_JSON_RESPONSE_BYTES)).Decode(&result); err != nil {
		return "", fmt.Errorf("decode auth response: %w", err)
	}

	if result.AuthToken == "" {
		return "", errors.New("auth returned empty auth_token")
	}

	c.token = result.AuthToken
	c.authScheme = DEFAULT_AUTH_SCHEME
	return c.token, nil
}

func (c *Client) Login(ctx context.Context, username, password string) error {
	_, err := c.postAuth(ctx, "/auth", map[string]string{
		"type":     "normal",
		"username": username,
		"password": password,
	})
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	return nil
}

func (c *Client) RefreshToken(ctx context.Context, refreshToken string) error {
	_, err := c.postAuth(ctx, "/auth/refresh", map[string]string{"refresh": refreshToken})
	if err != nil {
		return fmt.Errorf("refresh: %w", err)
	}
	return nil
}

func (c *Client) GetJSON(ctx context.Context, endpoint string, query url.Values) (any, ResponseMeta, error) {
	return c.doJSON(ctx, endpoint, query)
}

func (c *Client) PatchJSON(ctx context.Context, endpoint string, query url.Values, body any) (any, ResponseMeta, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("marshal PATCH body: %w", err)
	}

	u := c.endpointURL(endpoint, query)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("create Taiga PATCH request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.authorization())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("request Taiga: %w", err)
	}
	defer resp.Body.Close()

	if !isSuccess(resp.StatusCode) {
		return nil, ResponseMeta{}, fmt.Errorf("Taiga returned HTTP %d: %s", resp.StatusCode, errorMessage(resp))
	}

	result := any(nil)
	decoder := json.NewDecoder(io.LimitReader(resp.Body, MAX_JSON_RESPONSE_BYTES))
	if err := decoder.Decode(&result); err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("decode Taiga response: %w", err)
	}
	return result, responseMeta(resp), nil
}

func (c *Client) ListJSON(ctx context.Context, endpoint string, query url.Values, paginated bool) (any, ResponseMeta, error) {
	headers := map[string]string{}
	if !paginated {
		headers[DISABLE_PAGINATION_HEADER] = "True"
	}
	return c.doJSONWithHeaders(ctx, endpoint, query, headers)
}

func (c *Client) doJSON(ctx context.Context, endpoint string, query url.Values) (any, ResponseMeta, error) {
	return c.doJSONWithHeaders(ctx, endpoint, query, nil)
}

func (c *Client) authorization() string {
	return fmt.Sprintf("%s %s", c.authScheme, c.token)
}

func (c *Client) doJSONWithHeaders(ctx context.Context, endpoint string, query url.Values, extraHeaders map[string]string) (any, ResponseMeta, error) {
	u := c.endpointURL(endpoint, query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("create Taiga request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", c.authorization())
	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("request Taiga: %w", err)
	}
	defer resp.Body.Close()

	if !isSuccess(resp.StatusCode) {
		return nil, ResponseMeta{}, fmt.Errorf("Taiga returned HTTP %d: %s", resp.StatusCode, errorMessage(resp))
	}

	result := any(nil)
	decoder := json.NewDecoder(io.LimitReader(resp.Body, MAX_JSON_RESPONSE_BYTES))
	if err := decoder.Decode(&result); err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("decode Taiga response: %w", err)
	}
	return result, responseMeta(resp), nil
}

func (c *Client) endpointURL(endpoint string, query url.Values) *url.URL {
	u := *c.baseURL
	u.Path = fmt.Sprintf("%s/%s", strings.TrimRight(c.baseURL.Path, "/"), strings.TrimLeft(endpoint, "/"))
	u.RawPath = ""
	u.RawQuery = query.Encode()
	return &u
}

func responseMeta(resp *http.Response) ResponseMeta {
	return ResponseMeta{
		Paginated:    resp.Header.Get("x-paginated"),
		PaginatedBy:  resp.Header.Get("x-paginated-by"),
		Total:        resp.Header.Get("x-pagination-count"),
		CurrentPage:  resp.Header.Get("x-pagination-current"),
		NextPage:     resp.Header.Get("x-pagination-next"),
		PreviousPage: resp.Header.Get("x-pagination-prev"),
	}
}

func (c *Client) resolveSource(rawURL string) (*url.URL, error) {
	source, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("invalid attachment URL: %w", err)
	}

	if !source.IsAbs() {
		source = c.baseURL.ResolveReference(source)
	}

	invalid := false
	if source.Scheme != "http" {
		if source.Scheme != "https" {
			invalid = true
		}
	}

	if invalid {
		return nil, errors.New("attachment URL must use http or https")
	}

	return source, nil
}

func attachmentName(source *url.URL, filename string) string {
	name := filename
	if strings.TrimSpace(name) != "" {
		return sanitizeFilename(name)
	}

	name = source.Path
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}

	return sanitizeFilename(name)
}

func downloadTarget(dir, destPath string, name string) (string, string) {
	targetDir := dir
	if strings.TrimSpace(destPath) == "" {
		return targetDir, name
	}

	if filepath.Ext(strings.TrimSpace(destPath)) != "" {
		return filepath.Dir(strings.TrimSpace(destPath)), filepath.Base(strings.TrimSpace(destPath))
	}

	return strings.TrimSpace(destPath), name
}

func (c *Client) Download(ctx context.Context, rawURL string, filename string, destPath string) (DownloadResult, error) {
	source, err := c.resolveSource(rawURL)
	if err != nil {
		return DownloadResult{}, err
	}

	name := attachmentName(source, filename)
	targetDir, name := downloadTarget(c.downloadDir, destPath, name)

	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		return DownloadResult{}, fmt.Errorf("create download directory: %w", err)
	}

	destination, err := nextAvailablePath(targetDir, name)
	if err != nil {
		return DownloadResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.String(), nil)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("create attachment request: %w", err)
	}

	if sameOrigin(source, c.baseURL) {
		req.Header.Set("Authorization", c.authorization())
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("download attachment: %w", err)
	}
	defer resp.Body.Close()

	if !isSuccess(resp.StatusCode) {
		return DownloadResult{}, fmt.Errorf("attachment download returned HTTP %d: %s", resp.StatusCode, errorMessage(resp))
	}

	if resp.ContentLength > c.maxDownloadBytes {
		return DownloadResult{}, fmt.Errorf("attachment exceeds the %d-byte download limit", c.maxDownloadBytes)
	}

	tmp, err := os.CreateTemp(c.downloadDir, ".taiga-download-*")
	if err != nil {
		return DownloadResult{}, fmt.Errorf("create temporary download: %w", err)
	}

	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	bytesWritten, err := io.Copy(tmp, io.LimitReader(resp.Body, c.maxDownloadBytes+1))
	if err != nil {
		if cerr := tmp.Close(); cerr != nil {
			return DownloadResult{}, errors.Join(fmt.Errorf("write attachment: %w", err), cerr)
		}
		return DownloadResult{}, fmt.Errorf("write attachment: %w", err)
	}

	if bytesWritten > c.maxDownloadBytes {
		if cerr := tmp.Close(); cerr != nil {
			return DownloadResult{}, errors.Join(fmt.Errorf("attachment exceeds the %d-byte download limit", c.maxDownloadBytes), cerr)
		}
		return DownloadResult{}, fmt.Errorf("attachment exceeds the %d-byte download limit", c.maxDownloadBytes)
	}

	if err := tmp.Close(); err != nil {
		return DownloadResult{}, fmt.Errorf("close temporary download: %w", err)
	}

	if err := os.Rename(tmpName, destination); err != nil {
		return DownloadResult{}, fmt.Errorf("store downloaded attachment: %w", err)
	}

	return DownloadResult{
		Path:        destination,
		Bytes:       bytesWritten,
		ContentType: resp.Header.Get("Content-Type"),
		SourceURL:   source.String(),
	}, nil
}

func sameOrigin(a, b *url.URL) bool {
	sameScheme := strings.EqualFold(a.Scheme, b.Scheme)
	if !sameScheme {
		return false
	}
	return strings.EqualFold(a.Host, b.Host)
}

func nextAvailablePath(dir, filename string) (string, error) {
	candidate := filepath.Join(dir, filename)
	_, err := os.Stat(candidate)
	if errors.Is(err, os.ErrNotExist) {
		return candidate, nil
	}
	if err != nil {
		return "", fmt.Errorf("check download destination: %w", err)
	}

	ext := filepath.Ext(filename)
	stem := strings.TrimSuffix(filename, ext)
	for i := 1; i < MAX_DOWNLOAD_ATTEMPTS; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, i, ext))
		_, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("check download destination: %w", err)
		}
	}

	return "", errors.New("could not find an unused download destination")
}

func sanitizeFilename(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = path.Base(value)

	builder := strings.Builder{}
	for _, r := range value {
		switch {
		case r == 0:
			builder.WriteRune('_')
		case unicode.IsControl(r):
			builder.WriteRune('_')
		case strings.ContainsRune(`<>:"/\|?*`, r):
			builder.WriteRune('_')
		default:
			builder.WriteRune(r)
		}
	}

	value = strings.Trim(builder.String(), ". ")
	if value == "" {
		return DEFAULT_ATTACHMENT_NAME
	}
	if value == "." {
		return DEFAULT_ATTACHMENT_NAME
	}
	if value == ".." {
		return DEFAULT_ATTACHMENT_NAME
	}

	return value
}
