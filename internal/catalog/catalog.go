package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"bugfile-pan/internal/bigfile"
)

var (
	ErrInvalidPath  = errors.New("invalid path")
	ErrConflict     = errors.New("path conflict")
	ErrDuplicate    = errors.New("duplicate item")
	ErrNotDirectory = errors.New("not a directory")
)

// Snapshot is an immutable tree built from a BigFile share list.
type Snapshot struct {
	builtAt time.Time
	root    *node
}

type node struct {
	name     string
	dir      bool
	item     bigfile.ShareFileItem
	children map[string]*node
}

// Store provides atomic snapshot replacement.
type Store struct {
	value atomic.Value
}

// Load returns the current snapshot or nil.
func (s *Store) Load() *Snapshot {
	v := s.value.Load()
	if v == nil {
		return nil
	}
	return v.(*Snapshot)
}

// Store replaces the current snapshot atomically.
func (s *Store) Store(snapshot *Snapshot) {
	s.value.Store(snapshot)
}

// Build constructs a new immutable snapshot from share list items.
func Build(items []bigfile.ShareFileItem) (*Snapshot, error) {
	snap := &Snapshot{
		builtAt: time.Now().UTC(),
		root: &node{
			dir:      true,
			children: make(map[string]*node),
		},
	}
	for _, item := range items {
		if err := insertItem(snap.root, item); err != nil {
			return nil, err
		}
	}
	return snap, nil
}

func insertItem(root *node, item bigfile.ShareFileItem) error {
	// BigFile paths are relative (normally "./"). Keep lookup-path handling
	// separate by rejecting representations that are only valid for DAV roots.
	if item.Path == "" || strings.HasPrefix(item.Path, "/") {
		return fmt.Errorf("invalid item path %q: %w", item.Path, ErrInvalidPath)
	}
	segments, err := splitValidatedPath(item.Path)
	if err != nil {
		return fmt.Errorf("invalid item path %q: %w", item.Path, err)
	}
	if err := validateLeafName(item.Name); err != nil {
		return fmt.Errorf("invalid item name %q: %w", item.Name, err)
	}
	if !bigfile.IsBase62(item.Hash) {
		return fmt.Errorf("invalid item hash %q: %w", item.Hash, bigfile.ErrInvalidShareHash)
	}
	if item.Size < 0 {
		return fmt.Errorf("invalid item size %d", item.Size)
	}
	if item.Type == "" || strings.ContainsAny(item.Type, "\r\n\x00") {
		return fmt.Errorf("invalid item type %q", item.Type)
	}

	cur := root
	for _, seg := range segments {
		child, ok := cur.children[seg]
		if !ok {
			child = &node{name: seg, dir: true, children: make(map[string]*node)}
			cur.children[seg] = child
		} else if !child.dir {
			return fmt.Errorf("%w: %q is a file, not a directory", ErrConflict, seg)
		}
		cur = child
	}
	if _, ok := cur.children[item.Name]; ok {
		return fmt.Errorf("%w: %q", ErrDuplicate, joinDebugPath(segments, item.Name))
	}
	cur.children[item.Name] = &node{
		name: item.Name,
		dir:  false,
		item: item,
	}
	return nil
}

func validateLeafName(name string) error {
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "\x00") {
		return ErrInvalidPath
	}
	return nil
}

func joinDebugPath(dir []string, leaf string) string {
	parts := append(append([]string{}, dir...), leaf)
	return path.Join(parts...)
}

func splitValidatedPath(raw string) ([]string, error) {
	if raw == "" || raw == "." || raw == "./" || raw == "/" {
		return nil, nil
	}
	if strings.Contains(raw, "\x00") || strings.Contains(raw, "\\") {
		return nil, ErrInvalidPath
	}
	if strings.HasPrefix(raw, "/") {
		return nil, ErrInvalidPath
	}
	for strings.HasPrefix(raw, "./") {
		raw = strings.TrimPrefix(raw, "./")
	}
	raw = strings.TrimRight(raw, "/")
	if raw == "" || raw == "." {
		return nil, nil
	}
	parts := strings.Split(raw, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, ErrInvalidPath
		}
		segments = append(segments, part)
	}
	return segments, nil
}

// RootInfo returns the root directory info.
func (s *Snapshot) RootInfo() os.FileInfo {
	return newInfo(s.root, s.builtAt)
}

// Stat resolves a path within the snapshot.
func (s *Snapshot) Stat(name string) (os.FileInfo, error) {
	n, err := s.resolve(name)
	if err != nil {
		return nil, err
	}
	return newInfo(n, s.builtAt), nil
}

// Children resolves a directory and returns its entries.
func (s *Snapshot) Children(name string) ([]os.FileInfo, error) {
	n, err := s.resolve(name)
	if err != nil {
		return nil, err
	}
	if !n.dir {
		return nil, ErrNotDirectory
	}
	entries := make([]os.FileInfo, 0, len(n.children))
	for _, child := range n.children {
		entries = append(entries, newInfo(child, s.builtAt))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// Open resolves a path and returns a read-only file handle.
func (s *Snapshot) Open(name string) (*File, error) {
	n, err := s.resolve(name)
	if err != nil {
		return nil, err
	}
	return newFile(n, s.builtAt), nil
}

func (s *Snapshot) resolve(name string) (*node, error) {
	if name == "" || name == "/" {
		return s.root, nil
	}
	// x/net/webdav may pass one leading slash after removing a non-root
	// handler prefix. More than one is ambiguous and must not be normalized.
	if strings.HasPrefix(name, "//") {
		return nil, os.ErrNotExist
	}
	name = strings.TrimPrefix(name, "/")
	segments, err := splitValidatedPath(name)
	if err != nil {
		return nil, os.ErrNotExist
	}
	cur := s.root
	for _, seg := range segments {
		child, ok := cur.children[seg]
		if !ok {
			return nil, os.ErrNotExist
		}
		cur = child
	}
	return cur, nil
}

type info struct {
	node    *node
	modTime time.Time
}

func newInfo(n *node, modTime time.Time) *info {
	return &info{node: n, modTime: modTime}
}

func (i *info) Name() string { return i.node.name }
func (i *info) Size() int64 {
	if i.node.dir {
		return 0
	}
	return i.node.item.Size
}
func (i *info) Mode() os.FileMode {
	if i.node.dir {
		return os.ModeDir | 0o555
	}
	return 0o444
}
func (i *info) ModTime() time.Time { return i.modTime }
func (i *info) IsDir() bool        { return i.node.dir }
func (i *info) Sys() any           { return nil }
func (i *info) ContentType(ctx context.Context) (string, error) {
	if i.node.dir {
		return "", ErrNotDirectory
	}
	return i.node.item.Type, nil
}
func (i *info) ETag(ctx context.Context) (string, error) {
	if i.node.dir {
		return "", ErrNotDirectory
	}
	return `"` + i.node.item.Hash + `"`, nil
}
func (i *info) Hash() string {
	if i.node.dir {
		return ""
	}
	return i.node.item.Hash
}
func (i *info) MIME() string {
	if i.node.dir {
		return ""
	}
	return i.node.item.Type
}

type File struct {
	info     *info
	children []os.FileInfo
	childPos int
	closed   bool
}

func newFile(n *node, modTime time.Time) *File {
	f := &File{info: newInfo(n, modTime)}
	if n.dir {
		children := make([]os.FileInfo, 0, len(n.children))
		for _, child := range n.children {
			children = append(children, newInfo(child, modTime))
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		f.children = children
	}
	return f
}

func (f *File) Close() error { f.closed = true; return nil }

func (f *File) Read(p []byte) (int, error) {
	if f.closed {
		return 0, os.ErrClosed
	}
	return 0, os.ErrPermission
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	if f.closed {
		return 0, os.ErrClosed
	}
	return 0, os.ErrPermission
}

func (f *File) Readdir(count int) ([]os.FileInfo, error) {
	if !f.info.IsDir() {
		return nil, ErrNotDirectory
	}
	if f.closed {
		return nil, os.ErrClosed
	}
	if count <= 0 {
		children := append([]os.FileInfo(nil), f.children[f.childPos:]...)
		f.childPos = len(f.children)
		// os.File.Readdir requires a nil error for n <= 0 after a complete
		// read, including when the directory is empty.
		return children, nil
	}
	if f.childPos >= len(f.children) {
		return []os.FileInfo{}, io.EOF
	}
	end := f.childPos + count
	if end > len(f.children) {
		end = len(f.children)
	}
	children := append([]os.FileInfo(nil), f.children[f.childPos:end]...)
	f.childPos = end
	if len(children) < count {
		return children, io.EOF
	}
	return children, nil
}

func (f *File) Stat() (os.FileInfo, error) {
	if f.closed {
		return nil, os.ErrClosed
	}
	return f.info, nil
}

func (f *File) Write(p []byte) (int, error) {
	if f.closed {
		return 0, os.ErrClosed
	}
	return 0, os.ErrPermission
}
