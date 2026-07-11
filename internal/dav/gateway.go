package dav

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"bugfile-pan/internal/bigfile"
	"bugfile-pan/internal/catalog"
	"golang.org/x/net/webdav"
)

// Gateway is a read-only WebDAV handler backed by a BigFile share.
const (
	allowReadOnly       = "OPTIONS, GET, HEAD, PROPFIND"
	maxPropfindBodySize = 1 << 20
	writeLockXML        = `<D:lockentry xmlns:D="DAV:"><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype></D:lockentry>`
)

type Gateway struct {
	ShareHash string
	Prefix    string
	Client    *bigfile.Client
	Store     *catalog.Store

	handler *webdav.Handler
	prefix  string
}

// NewGateway creates a gateway and initializes the WebDAV handler.
func NewGateway(shareHash, prefix string, client *bigfile.Client, store *catalog.Store) (*Gateway, error) {
	if client == nil {
		return nil, fmt.Errorf("client is required")
	}
	if store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if !bigfile.IsBase62(shareHash) {
		return nil, bigfile.ErrInvalidShareHash
	}
	prefix, err := normalizePrefix(prefix)
	if err != nil {
		return nil, err
	}
	g := &Gateway{
		ShareHash: shareHash,
		Prefix:    prefix,
		Client:    client,
		Store:     store,
		prefix:    prefix,
	}
	g.handler = &webdav.Handler{
		Prefix:     prefix,
		FileSystem: fs{store: store},
		LockSystem: webdav.NewMemLS(),
	}
	return g, nil
}

// Refresh fetches the share list and atomically swaps in a new snapshot.
func (g *Gateway) Refresh(ctx context.Context) error {
	if g.Client == nil {
		return fmt.Errorf("client is required")
	}
	if g.Store == nil {
		return fmt.Errorf("store is required")
	}
	items, err := g.Client.FetchShareList(ctx, g.ShareHash)
	if err != nil {
		return err
	}
	snap, err := catalog.Build(items)
	if err != nil {
		return err
	}
	g.Store.Store(snap)
	return nil
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !g.matchPrefix(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodOptions {
		g.handleOptions(w, r)
		return
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		g.handleRead(w, r)
		return
	}
	if r.Method == "PROPFIND" {
		r.Body = http.MaxBytesReader(w, r.Body, maxPropfindBodySize)
		// x/net/webdav always emits an exclusive write lock entry in the
		// DAV:supportedlock live property. This gateway rejects LOCK, so remove
		// that entry while streaming the XML. The resulting empty property is
		// the RFC 4918 representation for a resource with no supported locks.
		filter := newLockFilterWriter(w)
		g.handler.ServeHTTP(filter, r)
		_ = filter.finish()
		return
	}
	g.methodNotAllowed(w)
}

func (g *Gateway) handleOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", allowReadOnly)
	w.Header().Set("DAV", "1")
	w.Header().Set("MS-Author-Via", "DAV")
	w.WriteHeader(http.StatusOK)
}

func (g *Gateway) handleRead(w http.ResponseWriter, r *http.Request) {
	rel, err := g.stripPrefix(r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	snap := g.Store.Load()
	if snap == nil {
		http.Error(w, "catalog unavailable", http.StatusServiceUnavailable)
		return
	}
	info, err := snap.Stat(rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if info.IsDir() {
		g.methodNotAllowed(w)
		return
	}
	meta, ok := info.(interface {
		os.FileInfo
		Hash() string
		MIME() string
	})
	if !ok {
		http.Error(w, "internal catalog type mismatch", http.StatusInternalServerError)
		return
	}
	location, err := g.Client.FileURL(meta.Hash(), meta.Name(), meta.MIME())
	if err != nil {
		http.Error(w, "failed to build file location", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Location", location)
	w.WriteHeader(http.StatusTemporaryRedirect)
}

func (g *Gateway) methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", allowReadOnly)
	w.WriteHeader(http.StatusMethodNotAllowed)
}

// lockFilterWriter removes x/net/webdav's hard-coded write-lock declaration
// without buffering a potentially large multistatus response.
type lockFilterWriter struct {
	dst     http.ResponseWriter
	pending []byte
	err     error
}

func newLockFilterWriter(dst http.ResponseWriter) *lockFilterWriter {
	return &lockFilterWriter{dst: dst}
}

func (w *lockFilterWriter) Header() http.Header { return w.dst.Header() }

func (w *lockFilterWriter) WriteHeader(statusCode int) { w.dst.WriteHeader(statusCode) }

func (w *lockFilterWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	w.pending = append(w.pending, p...)
	w.drain(false)
	if w.err != nil {
		return 0, w.err
	}
	return len(p), nil
}

func (w *lockFilterWriter) finish() error {
	w.drain(true)
	return w.err
}

func (w *lockFilterWriter) drain(final bool) {
	needle := []byte(writeLockXML)
	for w.err == nil {
		if index := bytes.Index(w.pending, needle); index >= 0 {
			w.writePending(index)
			if w.err != nil {
				return
			}
			w.pending = w.pending[len(needle):]
			continue
		}
		flushBytes := len(w.pending)
		if !final && flushBytes >= len(needle) {
			flushBytes -= len(needle) - 1
		} else if !final {
			flushBytes = 0
		}
		if flushBytes > 0 {
			w.writePending(flushBytes)
		}
		return
	}
}

func (w *lockFilterWriter) writePending(count int) {
	if count == 0 {
		return
	}
	written, err := w.dst.Write(w.pending[:count])
	w.pending = w.pending[written:]
	w.err = err
	if w.err == nil && written != count {
		w.err = io.ErrShortWrite
	}
}

func (g *Gateway) matchPrefix(p string) bool {
	if g.prefix == "/" {
		return strings.HasPrefix(p, "/")
	}
	return p == g.prefix || strings.HasPrefix(p, g.prefix+"/")
}

func (g *Gateway) stripPrefix(p string) (string, error) {
	if !g.matchPrefix(p) {
		return "", os.ErrNotExist
	}
	if g.prefix == "/" {
		if p == "/" {
			return "", nil
		}
		trimmed := strings.TrimPrefix(p, "/")
		if strings.HasPrefix(trimmed, "/") {
			return "", os.ErrNotExist
		}
		return trimmed, nil
	}
	if p == g.prefix {
		return "", nil
	}
	trimmed := strings.TrimPrefix(p, g.prefix+"/")
	if trimmed == p || strings.HasPrefix(trimmed, "/") {
		return "", os.ErrNotExist
	}
	return trimmed, nil
}

type fs struct {
	store *catalog.Store
}

func (f fs) Mkdir(ctx context.Context, name string, perm os.FileMode) error { return os.ErrPermission }
func (f fs) RemoveAll(ctx context.Context, name string) error               { return os.ErrPermission }
func (f fs) Rename(ctx context.Context, oldName, newName string) error      { return os.ErrPermission }

func (f fs) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	snap := f.store.Load()
	if snap == nil {
		return nil, os.ErrNotExist
	}
	return snap.Stat(name)
}

func (f fs) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	if flag != os.O_RDONLY {
		return nil, os.ErrPermission
	}
	snap := f.store.Load()
	if snap == nil {
		return nil, os.ErrNotExist
	}
	return snap.Open(name)
}

func normalizePrefix(prefix string) (string, error) {
	if prefix == "" {
		return "/", nil
	}
	if strings.ContainsAny(prefix, "\\\x00?#") {
		return "", fmt.Errorf("invalid DAV prefix")
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	for _, segment := range strings.Split(prefix, "/") {
		if segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid DAV prefix")
		}
	}
	clean := path.Clean(prefix)
	if clean == "." {
		return "/", nil
	}
	return clean, nil
}
