package catalog

import (
	"errors"
	"strings"
	"testing"

	"bugfile-pan/internal/bigfile"
)

func TestBuildAndLookup(t *testing.T) {
	snap, err := Build([]bigfile.ShareFileItem{
		{Name: "report.txt", Size: 5, Type: "text/plain", Hash: "Aa11", Path: "./"},
		{Name: "photo.png", Size: 9, Type: "image/png", Hash: "Bb22", Path: "docs/"},
		{Name: "notes.md", Size: 3, Type: "text/markdown", Hash: "Cc33", Path: "docs/sub/"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	root, err := snap.Stat("/")
	if err != nil {
		t.Fatalf("Stat root: %v", err)
	}
	if !root.IsDir() {
		t.Fatalf("root should be dir")
	}

	entries, err := snap.Children("")
	if err != nil {
		t.Fatalf("Children root: %v", err)
	}
	if len(entries) != 2 || entries[0].Name() != "docs" || entries[1].Name() != "report.txt" {
		t.Fatalf("unexpected root entries: %#v", entries)
	}

	docs, err := snap.Stat("docs")
	if err != nil {
		t.Fatalf("Stat docs: %v", err)
	}
	if !docs.IsDir() {
		t.Fatalf("docs should be dir")
	}

	photo, err := snap.Stat("docs/photo.png")
	if err != nil {
		t.Fatalf("Stat photo: %v", err)
	}
	if photo.IsDir() || photo.Size() != 9 || photo.Name() != "photo.png" {
		t.Fatalf("unexpected photo info: %#v", photo)
	}
	meta, ok := photo.(interface {
		Hash() string
		MIME() string
	})
	if !ok {
		t.Fatalf("photo info missing direct-link metadata")
	}
	if meta.Hash() != "Bb22" || meta.MIME() != "image/png" {
		t.Fatalf("unexpected metadata: hash=%q mime=%q", meta.Hash(), meta.MIME())
	}

	handle, err := snap.Open("docs")
	if err != nil {
		t.Fatalf("Open docs: %v", err)
	}
	children, err := handle.Readdir(0)
	if err != nil {
		t.Fatalf("Readdir docs: %v", err)
	}
	if len(children) != 2 || children[0].Name() != "photo.png" || children[1].Name() != "sub" {
		t.Fatalf("unexpected docs children: %#v", children)
	}
}

func TestBuildRejectsInvalidAndConflictingPaths(t *testing.T) {
	tests := []struct {
		name  string
		items []bigfile.ShareFileItem
	}{
		{
			name:  "absolute-path",
			items: []bigfile.ShareFileItem{{Name: "a.txt", Size: 1, Type: "text/plain", Hash: "Aa11", Path: "/abs/"}},
		},
		{
			name:  "path-traversal",
			items: []bigfile.ShareFileItem{{Name: "a.txt", Size: 1, Type: "text/plain", Hash: "Aa11", Path: "../abs/"}},
		},
		{
			name:  "empty-path",
			items: []bigfile.ShareFileItem{{Name: "a.txt", Size: 1, Type: "text/plain", Hash: "Aa11", Path: ""}},
		},
		{
			name:  "root-slash-path",
			items: []bigfile.ShareFileItem{{Name: "a.txt", Size: 1, Type: "text/plain", Hash: "Aa11", Path: "/"}},
		},
		{
			name:  "backslash-path",
			items: []bigfile.ShareFileItem{{Name: "a.txt", Size: 1, Type: "text/plain", Hash: "Aa11", Path: `docs\private`}},
		},
		{
			name:  "internal-dot-segment",
			items: []bigfile.ShareFileItem{{Name: "a.txt", Size: 1, Type: "text/plain", Hash: "Aa11", Path: "docs/./private"}},
		},
		{
			name:  "invalid-file-name",
			items: []bigfile.ShareFileItem{{Name: "../a.txt", Size: 1, Type: "text/plain", Hash: "Aa11", Path: "./"}},
		},
		{
			name:  "invalid-mime",
			items: []bigfile.ShareFileItem{{Name: "a.txt", Size: 1, Type: "text/plain\r\nBad: yes", Hash: "Aa11", Path: "./"}},
		},
		{
			name: "duplicate-file",
			items: []bigfile.ShareFileItem{
				{Name: "a.txt", Size: 1, Type: "text/plain", Hash: "Aa11", Path: "./"},
				{Name: "a.txt", Size: 2, Type: "text/plain", Hash: "Bb22", Path: "./"},
			},
		},
		{
			name: "file-directory-conflict",
			items: []bigfile.ShareFileItem{
				{Name: "docs", Size: 1, Type: "text/plain", Hash: "Aa11", Path: "./"},
				{Name: "a.txt", Size: 2, Type: "text/plain", Hash: "Bb22", Path: "docs/"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Build(tc.items)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, ErrInvalidPath) && !errors.Is(err, ErrConflict) && !errors.Is(err, ErrDuplicate) && !strings.Contains(err.Error(), "invalid item type") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestEmptyDirectoryReadAndAmbiguousLookup(t *testing.T) {
	snap, err := Build([]bigfile.ShareFileItem{})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := snap.Open("/")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := handle.Readdir(0)
	if err != nil {
		t.Fatalf("Readdir(0) on empty root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("got %d entries, want 0", len(entries))
	}
	if _, err := snap.Stat("//"); err == nil {
		t.Fatal("ambiguous double-slash lookup unexpectedly succeeded")
	}
}
