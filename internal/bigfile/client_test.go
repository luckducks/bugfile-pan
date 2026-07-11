package bigfile

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFileURLEncodesPathAndQuery(t *testing.T) {
	base, err := url.Parse("https://example.com/base")
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(base, time.Second)
	got, err := c.FileURL("Ab12Cd", "résumé 2026.txt", "text/plain; charset=utf-8")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://example.com/base/d/Ab12Cd/r%C3%A9sum%C3%A9%202026.txt?content-type=text%2Fplain%3B+charset%3Dutf-8"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFetchShareListValidation(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		maxBytes   int64
		maxItems   int
		wantErrSub string
	}{
		{
			name:   "ok",
			status: http.StatusOK,
			body:   `[{"name":"file.txt","size":5,"type":"text/plain","hash":"Aa11","path":"./"}]`,
		},
		{
			name:       "non-2xx",
			status:     http.StatusInternalServerError,
			body:       "oops",
			wantErrSub: "500 Internal Server Error",
		},
		{
			name:       "invalid-json",
			status:     http.StatusOK,
			body:       `{`,
			wantErrSub: "decode bigfile list",
		},
		{
			name:       "too-large",
			status:     http.StatusOK,
			body:       strings.Repeat("a", 10),
			maxBytes:   4,
			wantErrSub: "exceeds 4 bytes",
		},
		{
			name:       "too-many-items",
			status:     http.StatusOK,
			body:       `[{"name":"a.txt","size":1,"type":"text/plain","hash":"Aa11","path":"./"},{"name":"b.txt","size":1,"type":"text/plain","hash":"Bb22","path":"./"}]`,
			maxItems:   1,
			wantErrSub: "exceeds 1 items",
		},
		{
			name:       "invalid-field",
			status:     http.StatusOK,
			body:       `[{"name":"bad.txt","size":1,"type":"text/plain","hash":"bad/hash","path":"./"}]`,
			wantErrSub: "invalid bigfile list item",
		},
		{
			name:       "negative-size",
			status:     http.StatusOK,
			body:       `[{"name":"bad.txt","size":-1,"type":"text/plain","hash":"Aa11","path":"./"}]`,
			wantErrSub: "invalid size",
		},
		{
			name:       "null-is-not-an-array",
			status:     http.StatusOK,
			body:       `null`,
			wantErrSub: "expected a JSON array",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/d/Ab12Cd/list.json" {
					t.Fatalf("unexpected path %q", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			base, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			c := NewClient(base, time.Second)
			if tc.maxBytes != 0 {
				c.MaxBytes = tc.maxBytes
			}
			if tc.maxItems != 0 {
				c.MaxItems = tc.maxItems
			}
			items, err := c.FetchShareList(context.Background(), "Ab12Cd")
			if tc.wantErrSub == "" {
				if err != nil {
					t.Fatalf("FetchShareList error: %v", err)
				}
				if len(items) != 1 || items[0].Name != "file.txt" {
					t.Fatalf("unexpected items: %#v", items)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Fatalf("got err %v, want substring %q", err, tc.wantErrSub)
			}
		})
	}
}

func TestClientRejectsInvalidURLInputs(t *testing.T) {
	base, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(base, 0)

	for _, hash := range []string{"", "not/base62", "空"} {
		if _, err := client.ListURL(hash); err == nil {
			t.Fatalf("ListURL(%q) unexpectedly succeeded", hash)
		}
	}
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "a\x00b"} {
		if _, err := client.FileURL("Aa11", name, "text/plain"); err == nil {
			t.Fatalf("FileURL filename %q unexpectedly succeeded", name)
		}
	}
	for _, contentType := range []string{"", "text/plain\r\nX-Test: bad", "text/plain\x00"} {
		if _, err := client.FileURL("Aa11", "a.txt", contentType); err == nil {
			t.Fatalf("FileURL content type %q unexpectedly succeeded", contentType)
		}
	}

	badBase, err := url.Parse("file:///tmp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewClient(badBase, time.Second).ListURL("Aa11"); err == nil {
		t.Fatal("non-HTTP base URL unexpectedly accepted")
	}
}
