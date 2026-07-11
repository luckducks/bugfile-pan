package config

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"bugfile-pan/internal/bigfile"
)

// Config holds runtime settings loaded from environment variables.
type Config struct {
	ShareHash       string
	ListenAddr      string
	Prefix          string
	BaseURL         *url.URL
	UploadURL       *url.URL
	WebDir          string
	RefreshInterval time.Duration
	HTTPTimeout     time.Duration
	UploadTimeout   time.Duration
}

// Load parses the supported environment variables.
func Load() (Config, error) {
	prefix, err := normalizePrefix(envOrDefault("DAV_PREFIX", "/"))
	if err != nil {
		return Config{}, fmt.Errorf("parse DAV_PREFIX: %w", err)
	}
	cfg := Config{
		ListenAddr:      envOrDefault("LISTEN_ADDR", ":8080"),
		Prefix:          prefix,
		RefreshInterval: 5 * time.Minute,
		HTTPTimeout:     15 * time.Second,
		UploadTimeout:   30 * time.Minute,
		WebDir:          envOrDefault("WEB_DIR", "web/dist"),
	}
	if v := os.Getenv("BIGFILE_SHARE_HASH"); v != "" {
		cfg.ShareHash = v
	} else {
		return Config{}, fmt.Errorf("BIGFILE_SHARE_HASH is required")
	}
	if !bigfile.IsBase62(cfg.ShareHash) {
		return Config{}, fmt.Errorf("BIGFILE_SHARE_HASH must be base62")
	}
	baseURL := envOrDefault("BIGFILE_BASE_URL", "https://www.bigfile.net")
	u, err := url.Parse(baseURL)
	if err != nil {
		return Config{}, fmt.Errorf("parse BIGFILE_BASE_URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Config{}, fmt.Errorf("BIGFILE_BASE_URL must be an http(s) URL with a host")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Config{}, fmt.Errorf("BIGFILE_BASE_URL must not contain credentials, query, or fragment")
	}
	cfg.BaseURL = u
	uploadURL := envOrDefault("BIGFILE_UPLOAD_URL", "https://u1.bigfile.net/v1/upload")
	u, err = url.Parse(uploadURL)
	if err != nil {
		return Config{}, fmt.Errorf("parse BIGFILE_UPLOAD_URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Config{}, fmt.Errorf("BIGFILE_UPLOAD_URL must be an http(s) URL with a host")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Config{}, fmt.Errorf("BIGFILE_UPLOAD_URL must not contain credentials, query, or fragment")
	}
	cfg.UploadURL = u
	if v := os.Getenv("REFRESH_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("parse REFRESH_INTERVAL: %w", err)
		}
		if d < 0 {
			return Config{}, fmt.Errorf("REFRESH_INTERVAL must be non-negative")
		}
		cfg.RefreshInterval = d
	}
	if v := os.Getenv("HTTP_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("parse HTTP_TIMEOUT: %w", err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("HTTP_TIMEOUT must be positive")
		}
		cfg.HTTPTimeout = d
	}
	if v := os.Getenv("UPLOAD_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("parse UPLOAD_TIMEOUT: %w", err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("UPLOAD_TIMEOUT must be positive")
		}
		cfg.UploadTimeout = d
	}
	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func normalizePrefix(prefix string) (string, error) {
	if prefix == "" {
		return "/", nil
	}
	if strings.ContainsAny(prefix, "\\\x00?#") {
		return "", fmt.Errorf("must be a URL path without backslashes, NUL, query, or fragment")
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	for _, segment := range strings.Split(prefix, "/") {
		if segment == "." || segment == ".." {
			return "", fmt.Errorf("must not contain dot segments")
		}
	}
	clean := path.Clean(prefix)
	if clean == "." {
		return "/", nil
	}
	return clean, nil
}
