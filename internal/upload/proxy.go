package upload

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"bugfile-pan/internal/bigfile"
)

const (
	MaxChunkSize        = 96 << 20
	maxNextHeaderLength = 16 << 10
	maxResponseSize     = 1 << 20
)

// Proxy forwards the browser upload protocol to one fixed BigFile upload URL.
// It streams request bytes and never writes them to disk.
type Proxy struct {
	Target *url.URL
	Client *http.Client
}

func NewProxy(target *url.URL, client *http.Client) (*Proxy, error) {
	if target == nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return nil, fmt.Errorf("upload target must be an http(s) URL with a host")
	}
	if target.User != nil || target.RawQuery != "" || target.Fragment != "" {
		return nil, fmt.Errorf("upload target must not contain credentials, query, or fragment")
	}
	if client == nil {
		return nil, fmt.Errorf("http client is required")
	}
	copy := *target
	return &Proxy{Target: &copy, Client: client}, nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rangeLength, err := validateContentRange(r.Header.Get("Content-Range"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if r.ContentLength > MaxChunkSize {
		http.Error(w, "upload chunk exceeds 96 MiB", http.StatusRequestEntityTooLarge)
		return
	}
	if r.ContentLength >= 0 && r.ContentLength != rangeLength && !(r.ContentLength == 0 && rangeLength == 1) {
		http.Error(w, "Content-Range does not match request body length", http.StatusBadRequest)
		return
	}
	if hash := r.Header.Get("Hash"); hash != "" && !bigfile.IsBase62(hash) {
		http.Error(w, "Hash must be base62", http.StatusBadRequest)
		return
	}
	if next := r.Header.Get("Next"); len(next) > maxNextHeaderLength {
		http.Error(w, "Next header is too large", http.StatusBadRequest)
		return
	}

	body := http.MaxBytesReader(w, r.Body, MaxChunkSize)
	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, p.Target.String(), body)
	if err != nil {
		http.Error(w, "failed to create upstream request", http.StatusInternalServerError)
		return
	}
	upstream.ContentLength = r.ContentLength
	for _, name := range []string{"Content-Range", "Hash", "Next", "Content-Type"} {
		if value := r.Header.Get(name); value != "" {
			upstream.Header.Set(name, value)
		}
	}

	resp, err := p.Client.Do(upstream)
	if err != nil {
		http.Error(w, "BigFile upload request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		http.Error(w, "failed to read BigFile response", http.StatusBadGateway)
		return
	}
	if len(responseBody) > maxResponseSize {
		http.Error(w, "BigFile response is too large", http.StatusBadGateway)
		return
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
}

func validateContentRange(value string) (int64, error) {
	startText, endText, ok := strings.Cut(value, "-")
	if !ok || startText == "" || endText == "" || strings.Contains(endText, "-") {
		return 0, fmt.Errorf("invalid Content-Range")
	}
	start, err := strconv.ParseInt(startText, 10, 64)
	if err != nil || start < 0 {
		return 0, fmt.Errorf("invalid Content-Range")
	}
	end, err := strconv.ParseInt(endText, 10, 64)
	if err != nil || end < start {
		return 0, fmt.Errorf("invalid Content-Range")
	}
	length := end - start + 1
	if length > MaxChunkSize {
		return 0, fmt.Errorf("upload chunk exceeds 96 MiB")
	}
	return length, nil
}
