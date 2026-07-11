package config

import (
	"strings"
	"testing"
	"time"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BIGFILE_SHARE_HASH", "Ab12Cd")
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("DAV_PREFIX", "")
	t.Setenv("BIGFILE_BASE_URL", "")
	t.Setenv("BIGFILE_UPLOAD_URL", "")
	t.Setenv("REFRESH_INTERVAL", "")
	t.Setenv("HTTP_TIMEOUT", "")
	t.Setenv("UPLOAD_TIMEOUT", "")
	t.Setenv("WEB_DIR", "")
}

func TestLoadDefaults(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":8080" || cfg.Prefix != "/" {
		t.Fatalf("unexpected listen/prefix: %q %q", cfg.ListenAddr, cfg.Prefix)
	}
	if cfg.BaseURL.String() != "https://www.bigfile.net" {
		t.Fatalf("base URL=%q", cfg.BaseURL)
	}
	if cfg.UploadURL.String() != "https://u1.bigfile.net/v1/upload" {
		t.Fatalf("upload URL=%q", cfg.UploadURL)
	}
	if cfg.WebDir != "web/dist" {
		t.Fatalf("web dir=%q", cfg.WebDir)
	}
	if cfg.RefreshInterval != 5*time.Minute || cfg.HTTPTimeout != 15*time.Second || cfg.UploadTimeout != 30*time.Minute {
		t.Fatalf("unexpected durations: %v %v %v", cfg.RefreshInterval, cfg.HTTPTimeout, cfg.UploadTimeout)
	}
}

func TestLoadOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("DAV_PREFIX", "dav/")
	t.Setenv("BIGFILE_BASE_URL", "http://127.0.0.1:8081/base")
	t.Setenv("BIGFILE_UPLOAD_URL", "http://127.0.0.1:8082/v1/upload")
	t.Setenv("REFRESH_INTERVAL", "0")
	t.Setenv("HTTP_TIMEOUT", "2s")
	t.Setenv("UPLOAD_TIMEOUT", "3m")
	t.Setenv("WEB_DIR", "/srv/web")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Prefix != "/dav" || cfg.RefreshInterval != 0 || cfg.HTTPTimeout != 2*time.Second || cfg.UploadTimeout != 3*time.Minute || cfg.WebDir != "/srv/web" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "share hash", key: "BIGFILE_SHARE_HASH", value: "bad/hash", want: "base62"},
		{name: "base scheme", key: "BIGFILE_BASE_URL", value: "file:///tmp", want: "http(s)"},
		{name: "base credentials", key: "BIGFILE_BASE_URL", value: "https://user:pass@example.com", want: "credentials"},
		{name: "upload scheme", key: "BIGFILE_UPLOAD_URL", value: "file:///tmp", want: "http(s)"},
		{name: "upload query", key: "BIGFILE_UPLOAD_URL", value: "https://u1.bigfile.net/v1/upload?node=1", want: "query"},
		{name: "negative refresh", key: "REFRESH_INTERVAL", value: "-1s", want: "non-negative"},
		{name: "zero timeout", key: "HTTP_TIMEOUT", value: "0", want: "positive"},
		{name: "zero upload timeout", key: "UPLOAD_TIMEOUT", value: "0", want: "positive"},
		{name: "dot prefix", key: "DAV_PREFIX", value: "/dav/../private", want: "dot segments"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setRequiredEnv(t)
			t.Setenv(tc.key, tc.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want substring %q", err, tc.want)
			}
		})
	}
}
