package bigfile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	defaultListMaxBytes = 16 << 20
	defaultListMaxItems = 100_000
	defaultHTTPTimeout  = 15 * time.Second
)

var (
	ErrInvalidShareHash   = errors.New("invalid share hash")
	ErrInvalidFileName    = errors.New("invalid file name")
	ErrInvalidContentType = errors.New("invalid content type")
	ErrInvalidBaseURL     = errors.New("invalid base url")
)

// ShareFileItem mirrors the file object in BigFile share list.json.
type ShareFileItem struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Type string `json:"type"`
	Hash string `json:"hash"`
	Path string `json:"path"`
}

// Client fetches BigFile share metadata and builds direct file URLs.
type Client struct {
	BaseURL    *url.URL
	HTTPClient *http.Client
	MaxBytes   int64
	MaxItems   int
}

func NewClient(baseURL *url.URL, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	var baseCopy *url.URL
	if baseURL != nil {
		copy := *baseURL
		baseCopy = &copy
	}
	return &Client{
		BaseURL:    baseCopy,
		HTTPClient: &http.Client{Timeout: timeout},
		MaxBytes:   defaultListMaxBytes,
		MaxItems:   defaultListMaxItems,
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: defaultHTTPTimeout}
}

func (c *Client) fetchURL(shareHash, suffix string) (*url.URL, error) {
	if c.BaseURL == nil || (c.BaseURL.Scheme != "http" && c.BaseURL.Scheme != "https") || c.BaseURL.Host == "" || c.BaseURL.User != nil || c.BaseURL.RawQuery != "" || c.BaseURL.Fragment != "" {
		return nil, ErrInvalidBaseURL
	}
	if !IsBase62(shareHash) {
		return nil, ErrInvalidShareHash
	}
	if strings.Contains(suffix, "\x00") {
		return nil, ErrInvalidFileName
	}
	u := *c.BaseURL
	basePath := strings.TrimSuffix(u.Path, "/")
	u.Path = path.Join(basePath, "d", shareHash, suffix)
	u.RawPath = ""
	u.ForceQuery = false
	return &u, nil
}

// ListURL returns the URL used to fetch a share's list.json.
func (c *Client) ListURL(shareHash string) (string, error) {
	u, err := c.fetchURL(shareHash, "list.json")
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// FileURL returns a direct BigFile download URL.
func (c *Client) FileURL(shareHash, filename, contentType string) (string, error) {
	if filename == "" || strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "\x00") || filename == "." || filename == ".." {
		return "", ErrInvalidFileName
	}
	if contentType == "" || strings.ContainsAny(contentType, "\r\n\x00") {
		return "", ErrInvalidContentType
	}
	u, err := c.fetchURL(shareHash, filename)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("content-type", contentType)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// FetchShareList downloads and validates /d/{shareHash}/list.json.
func (c *Client) FetchShareList(ctx context.Context, shareHash string) ([]ShareFileItem, error) {
	u, err := c.fetchURL(shareHash, "list.json")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("bigfile list request failed: %s", resp.Status)
	}
	limit := c.MaxBytes
	if limit <= 0 {
		limit = defaultListMaxBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("bigfile list exceeds %d bytes", limit)
	}
	var items []ShareFileItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("decode bigfile list: %w", err)
	}
	if items == nil {
		return nil, fmt.Errorf("decode bigfile list: expected a JSON array")
	}
	maxItems := c.MaxItems
	if maxItems <= 0 {
		maxItems = defaultListMaxItems
	}
	if len(items) > maxItems {
		return nil, fmt.Errorf("bigfile list exceeds %d items", maxItems)
	}
	for i := range items {
		if err := validateItem(items[i]); err != nil {
			return nil, fmt.Errorf("invalid bigfile list item %d: %w", i, err)
		}
	}
	return items, nil
}

func validateItem(item ShareFileItem) error {
	if item.Name == "" || strings.Contains(item.Name, "/") || strings.Contains(item.Name, "\\") || strings.Contains(item.Name, "\x00") || item.Name == "." || item.Name == ".." {
		return ErrInvalidFileName
	}
	if !IsBase62(item.Hash) {
		return ErrInvalidShareHash
	}
	if item.Size < 0 {
		return fmt.Errorf("invalid size %d", item.Size)
	}
	if item.Type == "" || strings.ContainsAny(item.Type, "\r\n\x00") {
		return errors.New("invalid mime type")
	}
	if item.Path == "" {
		return errors.New("invalid path")
	}
	return nil
}

func IsBase62(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'A' && c <= 'Z':
		case c >= 'a' && c <= 'z':
		default:
			return false
		}
	}
	return true
}
