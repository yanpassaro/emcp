package kimai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	MAX_JSON_RESPONSE_BYTES  = 32 * 1024 * 1024
	MAX_ERROR_RESPONSE_BYTES = 8 * 1024
	LIST_ALL_LIMIT           = 1000
	DEFAULT_AUTH_SCHEME      = "Bearer"
	DEFAULT_TIMEOUT          = 30 * time.Second
)

type Config struct {
	BaseURL    string
	Token      string
	AuthScheme string
	HTTPClient *http.Client
}

type PaginationMeta struct {
	CurrentPage int `json:"currentPage"`
	PageSize    int `json:"pageSize"`
	TotalPages  int `json:"totalPages"`
	TotalCount  int `json:"totalCount"`
}

type Client struct {
	baseURL    *url.URL
	token      string
	authScheme string
	httpClient *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		return nil, errors.New("KIMAI_URL é obrigatório")
	}

	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("KIMAI_URL inválido: %w", err)
	}

	invalid := false
	if u.Scheme != "http" {
		if u.Scheme != "https" {
			invalid = true
		}
	}

	if invalid {
		return nil, errors.New("KIMAI_URL deve usar http ou https")
	}

	if u.Host == "" {
		return nil, errors.New("KIMAI_URL deve incluir um host")
	}

	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""

	authScheme := strings.TrimSpace(cfg.AuthScheme)
	if authScheme == "" {
		authScheme = DEFAULT_AUTH_SCHEME
	}

	if strings.ContainsAny(authScheme, " \t\r\n") {
		return nil, errors.New("AuthScheme não deve conter espaços")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DEFAULT_TIMEOUT}
	}

	return &Client{
		baseURL:    u,
		token:      strings.TrimSpace(cfg.Token),
		authScheme: authScheme,
		httpClient: httpClient,
	}, nil
}

func (c *Client) GetJSON(ctx context.Context, endpoint string, query url.Values) (any, error) {
	return c.doJSON(ctx, http.MethodGet, endpoint, query, nil)
}

func (c *Client) ListJSON(ctx context.Context, endpoint string, query url.Values, paginated bool) ([]any, PaginationMeta, error) {
	q := url.Values{}
	for k, vs := range query {
		q[k] = vs
	}

	if !paginated {
		q.Set("page", "1")
		q.Set("limit", strconv.Itoa(LIST_ALL_LIMIT))
	}

	raw, err := c.doJSON(ctx, http.MethodGet, endpoint, q, nil)
	if err != nil {
		return nil, PaginationMeta{}, err
	}
	return extractCollection(raw)
}

func (c *Client) PostJSON(ctx context.Context, endpoint string, query url.Values, body any) (any, error) {
	return c.doJSON(ctx, http.MethodPost, endpoint, query, body)
}

func (c *Client) PatchJSON(ctx context.Context, endpoint string, query url.Values, body any) (any, error) {
	return c.doJSON(ctx, http.MethodPatch, endpoint, query, body)
}

func (c *Client) DeleteJSON(ctx context.Context, endpoint string, query url.Values) error {
	_, err := c.doJSON(ctx, http.MethodDelete, endpoint, query, nil)
	return err
}

func (c *Client) TestConnection(ctx context.Context) error {
	_, err := c.GetJSON(ctx, "/api/users/me", nil)
	return err
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, query url.Values, body any) (any, error) {
	u := c.endpointURL(endpoint, query)

	reqBody := io.Reader(nil)
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal do corpo da requisição: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("criar requisição: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("%s %s", c.authScheme, c.token))
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requisição ao Kimai: %w", err)
	}
	defer resp.Body.Close()

	failed := false
	if resp.StatusCode < http.StatusOK {
		failed = true
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		failed = true
	}

	if failed {
		return nil, fmt.Errorf("Kimai retornou HTTP %d: %s", resp.StatusCode, errorMessage(resp))
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	dec := json.NewDecoder(io.LimitReader(resp.Body, MAX_JSON_RESPONSE_BYTES))
	result := any(nil)
	if err := dec.Decode(&result); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, fmt.Errorf("decodificar resposta do Kimai: %w", err)
	}

	return result, nil
}

func errorMessage(resp *http.Response) string {
	detail, err := io.ReadAll(io.LimitReader(resp.Body, MAX_ERROR_RESPONSE_BYTES))
	if err != nil {
		return http.StatusText(resp.StatusCode)
	}

	msg := strings.TrimSpace(string(detail))
	if msg == "" {
		return http.StatusText(resp.StatusCode)
	}

	return msg
}

func (c *Client) endpointURL(endpoint string, query url.Values) *url.URL {
	u := *c.baseURL
	u.Path = fmt.Sprintf("%s/%s", strings.TrimRight(c.baseURL.Path, "/"), strings.TrimLeft(endpoint, "/"))
	u.RawPath = ""
	u.RawQuery = query.Encode()
	return &u
}

func extractCollection(raw any) ([]any, PaginationMeta, error) {
	switch v := raw.(type) {
	case nil:
		return nil, PaginationMeta{}, nil
	case []any:
		return v, PaginationMeta{}, nil
	case map[string]any:
		items, ok := v["data"].([]any)
		if !ok {
			items = []any{}
		}
		return items, paginationMeta(v), nil
	default:
		return nil, PaginationMeta{}, fmt.Errorf("resposta de coleção inesperada: %T", raw)
	}
}

func paginationMeta(v map[string]any) PaginationMeta {
	m, ok := v["meta"].(map[string]any)
	if !ok {
		return PaginationMeta{}
	}

	b, err := json.Marshal(m)
	if err != nil {
		return PaginationMeta{}
	}

	meta := PaginationMeta{}
	if err := json.Unmarshal(b, &meta); err != nil {
		return PaginationMeta{}
	}

	return meta
}
