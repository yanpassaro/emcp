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
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	defaultBaseURL        = "https://api.taiga.io"
	defaultMaxDownload    = 100 * 1024 * 1024
	maxJSONResponseBytes  = 32 * 1024 * 1024
	maxErrorResponseBytes = 8 * 1024
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
		base = defaultBaseURL
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid TAIGA_URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("TAIGA_URL must use http or https")
	}
	if u.Host == "" {
		return nil, errors.New("TAIGA_URL must include a host")
	}
	if u.RawQuery != "" || u.Fragment != "" {
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
		authScheme = "Bearer"
	}
	if strings.ContainsAny(authScheme, " \t\r\n") {
		return nil, errors.New("TAIGA_AUTH_SCHEME must not contain whitespace")
	}

	downloadDir := strings.TrimSpace(cfg.DownloadDir)
	if downloadDir == "" {
		downloadDir = filepath.Join(UserLocalDir(), "mcp", "taiga", "downloads")
	}
	downloadDir, err = filepath.Abs(downloadDir)
	if err != nil {
		return nil, fmt.Errorf("invalid download directory: %w", err)
	}
	maxDownloadBytes := cfg.MaxDownloadBytes
	if maxDownloadBytes <= 0 {
		maxDownloadBytes = defaultMaxDownload
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	return &Client{
		baseURL:          u,
		token:            token,
		authScheme:       authScheme,
		httpClient:       httpClient,
		downloadDir:      downloadDir,
		maxDownloadBytes: maxDownloadBytes,
	}, nil
}

func UserLocalDir() string {
	if home := os.Getenv("USERPROFILE"); home != "" {
		return filepath.Join(home, ".local", "share")
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "share")
	}
	return filepath.Join(".", "taiga-mcp")
}

func (c *Client) Login(ctx context.Context, username, password string) error {
	body, err := json.Marshal(map[string]string{
		"type":     "normal",
		"username": username,
		"password": password,
	})
	if err != nil {
		return fmt.Errorf("marshal login payload: %w", err)
	}
	u := c.endpointURL("/auth", nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBytes))
		message := strings.TrimSpace(string(detail))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("login failed (HTTP %d): %s", resp.StatusCode, message)
	}
	var result struct {
		AuthToken string `json:"auth_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONResponseBytes)).Decode(&result); err != nil {
		return fmt.Errorf("decode login response: %w", err)
	}
	if result.AuthToken == "" {
		return errors.New("login returned empty auth_token")
	}
	c.token = result.AuthToken
	c.authScheme = "Bearer"
	return nil
}

func (c *Client) RefreshToken(ctx context.Context, refreshToken string) error {
	body, err := json.Marshal(map[string]string{"refresh": refreshToken})
	if err != nil {
		return fmt.Errorf("marshal refresh payload: %w", err)
	}
	u := c.endpointURL("/auth/refresh", nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBytes))
		message := strings.TrimSpace(string(detail))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("refresh failed (HTTP %d): %s", resp.StatusCode, message)
	}
	var result struct {
		AuthToken string `json:"auth_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONResponseBytes)).Decode(&result); err != nil {
		return fmt.Errorf("decode refresh response: %w", err)
	}
	if result.AuthToken == "" {
		return errors.New("refresh returned empty auth_token")
	}
	c.token = result.AuthToken
	c.authScheme = "Bearer"
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
	req.Header.Set("Authorization", c.authScheme+" "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("request Taiga: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBytes))
		message := strings.TrimSpace(string(detail))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, ResponseMeta{}, fmt.Errorf("Taiga returned HTTP %d: %s", resp.StatusCode, message)
	}

	var result any
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxJSONResponseBytes))
	if err := decoder.Decode(&result); err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("decode Taiga response: %w", err)
	}
	return result, responseMeta(resp), nil
}

func (c *Client) ListJSON(ctx context.Context, endpoint string, query url.Values, paginated bool) (any, ResponseMeta, error) {
	headers := map[string]string{}
	if !paginated {
		headers["x-disable-pagination"] = "True"
	}
	return c.doJSONWithHeaders(ctx, endpoint, query, headers)
}

func (c *Client) doJSON(ctx context.Context, endpoint string, query url.Values) (any, ResponseMeta, error) {
	return c.doJSONWithHeaders(ctx, endpoint, query, nil)
}

func (c *Client) doJSONWithHeaders(ctx context.Context, endpoint string, query url.Values, extraHeaders map[string]string) (any, ResponseMeta, error) {
	u := c.endpointURL(endpoint, query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("create Taiga request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", c.authScheme+" "+c.token)
	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("request Taiga: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBytes))
		message := strings.TrimSpace(string(detail))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, ResponseMeta{}, fmt.Errorf("Taiga returned HTTP %d: %s", resp.StatusCode, message)
	}

	var result any
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxJSONResponseBytes))
	if err := decoder.Decode(&result); err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("decode Taiga response: %w", err)
	}
	return result, responseMeta(resp), nil
}

func (c *Client) endpointURL(endpoint string, query url.Values) *url.URL {
	u := *c.baseURL
	u.Path = strings.TrimRight(c.baseURL.Path, "/") + "/" + strings.TrimLeft(endpoint, "/")
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

func (c *Client) Download(ctx context.Context, rawURL string, filename string, destPath string) (DownloadResult, error) {
	source, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return DownloadResult{}, fmt.Errorf("invalid attachment URL: %w", err)
	}
	if !source.IsAbs() {
		source = c.baseURL.ResolveReference(source)
	}
	if source.Scheme != "http" && source.Scheme != "https" {
		return DownloadResult{}, errors.New("attachment URL must use http or https")
	}

	name := filename
	if strings.TrimSpace(name) == "" {
		name = source.Path
		if decoded, decodeErr := url.PathUnescape(name); decodeErr == nil {
			name = decoded
		}
	}
	name = sanitizeFilename(name)

	targetDir := c.downloadDir
	if strings.TrimSpace(destPath) != "" {
		if filepath.Ext(strings.TrimSpace(destPath)) != "" {
			targetDir = filepath.Dir(strings.TrimSpace(destPath))
			name = filepath.Base(strings.TrimSpace(destPath))
		} else {
			targetDir = strings.TrimSpace(destPath)
		}
	}
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
		req.Header.Set("Authorization", c.authScheme+" "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("download attachment: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseBytes))
		message := strings.TrimSpace(string(detail))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return DownloadResult{}, fmt.Errorf("attachment download returned HTTP %d: %s", resp.StatusCode, message)
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
		_ = tmp.Close()
		return DownloadResult{}, fmt.Errorf("write attachment: %w", err)
	}
	if bytesWritten > c.maxDownloadBytes {
		_ = tmp.Close()
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
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

func nextAvailablePath(dir, filename string) (string, error) {
	candidate := filepath.Join(dir, filename)
	if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
		return candidate, nil
	} else if err != nil {
		return "", fmt.Errorf("check download destination: %w", err)
	}

	ext := filepath.Ext(filename)
	stem := strings.TrimSuffix(filename, ext)
	for i := 1; i < 10000; i++ {
		candidate = filepath.Join(dir, stem+"-"+strconv.Itoa(i)+ext)
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("check download destination: %w", err)
		}
	}
	return "", errors.New("could not find an unused download destination")
}

func sanitizeFilename(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = path.Base(value)
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r == 0 || unicode.IsControl(r):
			builder.WriteRune('_')
		case strings.ContainsRune(`<>:"/\\|?*`, r):
			builder.WriteRune('_')
		default:
			builder.WriteRune(r)
		}
	}
	value = strings.Trim(builder.String(), ". ")
	if value == "" || value == "." || value == ".." {
		return "attachment"
	}
	return value
}
