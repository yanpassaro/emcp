package gitlab

import (
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
	DEFAULT_BASE_URL         = "https://gitlab.com"
	MAX_JSON_RESPONSE_BYTES  = 32 * 1024 * 1024
	MAX_ERROR_RESPONSE_BYTES = 8 * 1024
	DEFAULT_TIMEOUT          = 60 * time.Second
	DEFAULT_MAX_ITEMS        = 50
	MAX_PER_PAGE             = 100
	MAX_PAGES                = 200
)

type Config struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
}

type ResponseMeta struct {
	Total      string
	TotalPages string
	Page       string
	PerPage    string
}

func NewClient(cfg Config) (*Client, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		base = DEFAULT_BASE_URL
	}

	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid GITLAB_URL: %w", err)
	}

	invalid := false
	if u.Scheme != "http" {
		if u.Scheme != "https" {
			invalid = true
		}
	}

	if invalid {
		return nil, errors.New("GITLAB_URL must use http or https")
	}

	if u.Host == "" {
		return nil, errors.New("GITLAB_URL must include a host")
	}

	basePath := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(basePath, "/api/v4") {
		basePath += "/api/v4"
	}

	u.Path = basePath
	u.RawPath = ""

	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, errors.New("GITLAB_TOKEN is required")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DEFAULT_TIMEOUT}
	}

	return &Client{baseURL: u, token: token, httpClient: httpClient}, nil
}

func (c *Client) endpointURL(endpoint string, query url.Values) *url.URL {
	u := *c.baseURL
	u.Path = fmt.Sprintf("%s/%s", strings.TrimRight(c.baseURL.Path, "/"), strings.TrimLeft(endpoint, "/"))
	u.RawPath = ""
	u.RawQuery = query.Encode()
	return &u
}

func (c *Client) do(ctx context.Context, endpoint string, query url.Values) (any, ResponseMeta, error) {
	u := c.endpointURL(endpoint, query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("create GitLab request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("request GitLab: %w", err)
	}
	defer resp.Body.Close()

	failed := false
	if resp.StatusCode < 200 {
		failed = true
	}

	if resp.StatusCode >= 300 {
		failed = true
	}

	if failed {
		message := readErrorMessage(resp)
		return nil, ResponseMeta{}, fmt.Errorf("GitLab returned HTTP %d: %s", resp.StatusCode, message)
	}

	result := any(nil)
	decoder := json.NewDecoder(io.LimitReader(resp.Body, MAX_JSON_RESPONSE_BYTES))
	if err := decoder.Decode(&result); err != nil {
		return nil, ResponseMeta{}, fmt.Errorf("decode GitLab response: %w", err)
	}

	return result, responseMeta(resp), nil
}

func readErrorMessage(resp *http.Response) string {
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

func responseMeta(resp *http.Response) ResponseMeta {
	return ResponseMeta{
		Total:      resp.Header.Get("X-Total"),
		TotalPages: resp.Header.Get("X-Total-Pages"),
		Page:       resp.Header.Get("X-Page"),
		PerPage:    resp.Header.Get("X-Per-Page"),
	}
}

func (c *Client) List(ctx context.Context, endpoint string, query url.Values, max int) ([]any, error) {
	if max <= 0 {
		max = DEFAULT_MAX_ITEMS
	}

	perPage := max
	if perPage > MAX_PER_PAGE {
		perPage = MAX_PER_PAGE
	}

	query.Set("per_page", strconv.Itoa(perPage))
	collected := make([]any, 0, max)
	page := 1

	for page <= MAX_PAGES {
		query.Set("page", strconv.Itoa(page))
		raw, _, err := c.do(ctx, endpoint, query)
		if err != nil {
			if len(collected) > 0 {
				return collected, nil
			}
			return nil, err
		}

		items, ok := raw.([]any)
		if !ok {
			break
		}

		collected = append(collected, items...)
		if len(collected) >= max {
			if len(collected) > max {
				collected = collected[:max]
			}
			break
		}

		if len(items) < perPage {
			break
		}

		page++
	}

	return collected, nil
}

func (c *Client) Get(ctx context.Context, endpoint string, query url.Values) (any, error) {
	raw, _, err := c.do(ctx, endpoint, query)
	return raw, err
}
