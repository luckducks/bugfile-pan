package dav

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bugfile-pan/internal/bigfile"
	"bugfile-pan/internal/catalog"
)

type backendState struct {
	mu         sync.Mutex
	listBody   string
	listStatus int
	listHits   atomic.Int32
	fileHits   atomic.Int32
}

func (s *backendState) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/list.json"):
		s.listHits.Add(1)
		s.mu.Lock()
		body := s.listBody
		status := s.listStatus
		s.mu.Unlock()
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	default:
		if strings.HasPrefix(r.URL.Path, "/d/") {
			s.fileHits.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "unexpected file fetch")
			return
		}
		http.NotFound(w, r)
	}
}

func (s *backendState) setList(body string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listBody = body
	s.listStatus = status
}

func TestGatewayReadOnlyBehavior(t *testing.T) {
	backend := &backendState{}
	backend.setList(`[
		{"name":"report.txt","size":5,"type":"text/plain; charset=utf-8","hash":"Aa11","path":"./"},
		{"name":"résumé 2026.txt","size":7,"type":"text/plain; charset=utf-8","hash":"Bb22","path":"docs/"},
		{"name":"nested.bin","size":3,"type":"application/octet-stream","hash":"Cc33","path":"docs/sub/"}
	]`, http.StatusOK)

	upstream := httptest.NewServer(backend)
	defer upstream.Close()

	baseURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := bigfile.NewClient(baseURL, time.Second)
	store := &catalog.Store{}
	g, err := NewGateway("Ab12Cd", "/dav", client, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	server := httptest.NewServer(g)
	defer server.Close()
	clientHTTP := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	t.Run("options", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodOptions, server.URL+"/dav", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := clientHTTP.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status=%d", resp.StatusCode)
		}
		if got := resp.Header.Get("Allow"); got != "OPTIONS, GET, HEAD, PROPFIND" {
			t.Fatalf("allow=%q", got)
		}
		if got := resp.Header.Get("DAV"); got != "1" {
			t.Fatalf("dav=%q", got)
		}
	})

	t.Run("propfind-depth-1-root", func(t *testing.T) {
		req, err := http.NewRequest("PROPFIND", server.URL+"/dav/", strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Depth", "1")
		resp, err := clientHTTP.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusMultiStatus {
			t.Fatalf("status=%d body=%s", resp.StatusCode, body)
		}
		if !strings.Contains(string(body), "/dav/") || !strings.Contains(string(body), "/dav/docs/") || !strings.Contains(string(body), "/dav/report.txt") {
			t.Fatalf("propfind body missing expected root hrefs:\n%s", body)
		}
		if strings.Contains(string(body), "<D:write") {
			t.Fatalf("read-only PROPFIND advertised write locks:\n%s", body)
		}
	})

	t.Run("propfind-depth-1-directory", func(t *testing.T) {
		req, err := http.NewRequest("PROPFIND", server.URL+"/dav/docs/", strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Depth", "1")
		resp, err := clientHTTP.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusMultiStatus {
			t.Fatalf("status=%d body=%s", resp.StatusCode, body)
		}
		encodedName := url.PathEscape("résumé 2026.txt")
		if !strings.Contains(string(body), "/dav/docs/") || !strings.Contains(string(body), "/dav/docs/sub/") || !strings.Contains(string(body), "/dav/docs/"+encodedName) {
			t.Fatalf("propfind body missing expected directory hrefs:\n%s", body)
		}
	})

	t.Run("get-head-redirect", func(t *testing.T) {
		path := "/dav/docs/" + url.PathEscape("résumé 2026.txt")
		req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := clientHTTP.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusTemporaryRedirect {
			t.Fatalf("status=%d", resp.StatusCode)
		}
		loc := resp.Header.Get("Location")
		u, err := url.Parse(loc)
		if err != nil {
			t.Fatalf("parse location: %v", err)
		}
		if u.Path != "/d/Bb22/résumé 2026.txt" {
			t.Fatalf("path=%q raw=%q", u.Path, loc)
		}
		if got := u.Query().Get("content-type"); got != "text/plain; charset=utf-8" {
			t.Fatalf("content-type=%q raw=%q", got, loc)
		}
		if !strings.Contains(loc, "r%C3%A9sum%C3%A9%202026.txt") || !strings.Contains(loc, "content-type=text%2Fplain%3B+charset%3Dutf-8") {
			t.Fatalf("location not encoded as expected: %q", loc)
		}

		req, err = http.NewRequest(http.MethodHead, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err = clientHTTP.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusTemporaryRedirect {
			t.Fatalf("head status=%d", resp.StatusCode)
		}
		if got := resp.Header.Get("Location"); got != loc {
			t.Fatalf("head location=%q want %q", got, loc)
		}
	})

	t.Run("directory-get-is-405", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/dav/docs", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := clientHTTP.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d", resp.StatusCode)
		}
		if got := resp.Header.Get("Allow"); got != "OPTIONS, GET, HEAD, PROPFIND" {
			t.Fatalf("allow=%q", got)
		}
	})

	t.Run("write-methods-are-forbidden", func(t *testing.T) {
		for _, method := range []string{http.MethodPut, http.MethodDelete, http.MethodPost, "MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK", "PROPPATCH"} {
			t.Run(method, func(t *testing.T) {
				req, err := http.NewRequest(method, server.URL+"/dav/docs/"+url.PathEscape("résumé 2026.txt"), strings.NewReader("body"))
				if err != nil {
					t.Fatal(err)
				}
				resp, err := clientHTTP.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusMethodNotAllowed {
					t.Fatalf("status=%d", resp.StatusCode)
				}
			})
		}
	})

	t.Run("missing-and-prefix-paths", func(t *testing.T) {
		for _, requestPath := range []string{"/outside/report.txt", "/dav/missing.txt", "/dav//report.txt"} {
			resp, err := clientHTTP.Get(server.URL + requestPath)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("path %q status=%d, want 404", requestPath, resp.StatusCode)
			}
		}
	})

	t.Run("propfind-depth-0-file", func(t *testing.T) {
		req, err := http.NewRequest("PROPFIND", server.URL+"/dav/report.txt", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Depth", "0")
		resp, err := clientHTTP.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusMultiStatus || !strings.Contains(string(body), `"Aa11"`) {
			t.Fatalf("status=%d body=%s", resp.StatusCode, body)
		}
	})

	if got := backend.fileHits.Load(); got != 0 {
		t.Fatalf("unexpected upstream file fetches: %d", got)
	}
}

func TestLockFilterAcrossWriteBoundaries(t *testing.T) {
	recorder := httptest.NewRecorder()
	filter := newLockFilterWriter(recorder)
	payload := "before" + writeLockXML + "after"
	for _, part := range []string{payload[:17], payload[17:53], payload[53:]} {
		if _, err := filter.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	if err := filter.finish(); err != nil {
		t.Fatal(err)
	}
	if got := recorder.Body.String(); got != "beforeafter" {
		t.Fatalf("filtered output=%q", got)
	}
}

func TestGatewayRejectsInvalidConstruction(t *testing.T) {
	baseURL, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	client := bigfile.NewClient(baseURL, time.Second)
	store := &catalog.Store{}
	if _, err := NewGateway("bad/hash", "/", client, store); err == nil {
		t.Fatal("invalid share hash unexpectedly accepted")
	}
	if _, err := NewGateway("Aa11", "/dav/../private", client, store); err == nil {
		t.Fatal("unsafe prefix unexpectedly accepted")
	}
}

func TestGatewayEmptyCatalogPropfind(t *testing.T) {
	baseURL, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	store := &catalog.Store{}
	snapshot, err := catalog.Build([]bigfile.ShareFileItem{})
	if err != nil {
		t.Fatal(err)
	}
	store.Store(snapshot)
	gateway, err := NewGateway("Aa11", "/", bigfile.NewClient(baseURL, time.Second), store)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("PROPFIND", "http://example.com/", nil)
	req.Header.Set("Depth", "1")
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, req)
	if response.Code != http.StatusMultiStatus {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestGatewayRefreshFailureKeepsOldSnapshot(t *testing.T) {
	backend := &backendState{}
	backend.setList(`[{"name":"report.txt","size":5,"type":"text/plain","hash":"Aa11","path":"./"}]`, http.StatusOK)
	upstream := httptest.NewServer(backend)
	defer upstream.Close()

	baseURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := bigfile.NewClient(baseURL, time.Second)
	store := &catalog.Store{}
	g, err := NewGateway("Ab12Cd", "/", client, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Refresh(context.Background()); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}

	backend.setList("not-json", http.StatusOK)
	if err := g.Refresh(context.Background()); err == nil {
		t.Fatal("expected refresh error")
	}

	server := httptest.NewServer(g)
	defer server.Close()
	clientHTTP := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/report.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := clientHTTP.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); !strings.Contains(got, "/d/Aa11/report.txt") {
		t.Fatalf("unexpected location %q", got)
	}
}
