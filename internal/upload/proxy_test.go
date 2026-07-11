package upload

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestProxyForwardsUploadProtocol(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/upload" {
			t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		for name, want := range map[string]string{
			"Content-Range": "3-4",
			"Hash":          "Ab12Cd",
			"Next":          "token",
			"Content-Type":  "application/octet-stream",
		} {
			if got := r.Header.Get(name); got != want {
				t.Errorf("%s=%q, want %q", name, got, want)
			}
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "LO" {
			t.Fatalf("body=%q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":true,"uri":"/d/Ab12Cd"}`)
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL + "/v1/upload")
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(target, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/upload", strings.NewReader("LO"))
	req.Header.Set("Content-Range", "3-4")
	req.Header.Set("Hash", "Ab12Cd")
	req.Header.Set("Next", "token")
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control=%q", rec.Header().Get("Cache-Control"))
	}
}

func TestProxyRejectsInvalidRequests(t *testing.T) {
	target, _ := url.Parse("https://u1.bigfile.net/v1/upload")
	proxy, err := NewProxy(target, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		method string
		range_ string
		hash   string
		body   string
		status int
	}{
		{name: "method", method: http.MethodGet, status: http.StatusMethodNotAllowed},
		{name: "missing range", method: http.MethodPost, body: "x", status: http.StatusBadRequest},
		{name: "bad range", method: http.MethodPost, range_: "2-1", body: "x", status: http.StatusBadRequest},
		{name: "bad hash", method: http.MethodPost, range_: "0-0", hash: "bad/hash", body: "x", status: http.StatusBadRequest},
		{name: "length mismatch", method: http.MethodPost, range_: "0-2", body: "x", status: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/upload", strings.NewReader(tc.body))
			if tc.range_ != "" {
				req.Header.Set("Content-Range", tc.range_)
			}
			if tc.hash != "" {
				req.Header.Set("Hash", tc.hash)
			}
			rec := httptest.NewRecorder()
			proxy.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d, want %d; body=%q", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}

func TestValidateContentRange(t *testing.T) {
	for _, value := range []string{"", "bytes 0-1/2", "-1", "1-", "x-1", "2-1", "1-2-3"} {
		if _, err := validateContentRange(value); err == nil {
			t.Errorf("validateContentRange(%q) succeeded", value)
		}
	}
	if got, err := validateContentRange("96-191"); err != nil || got != 96 {
		t.Fatalf("length=%d error=%v", got, err)
	}
}
