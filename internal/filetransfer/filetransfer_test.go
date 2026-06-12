package filetransfer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/mmazur/cmdproxy/internal/config"
	"github.com/mmazur/cmdproxy/internal/policy/argmatch"
)

func TestBuildRequestRelativeFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("doc.md", []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}

	bundle, err := BuildRequest("gws", []string{"drive", "files", "create", "--upload", "./doc.md"}, []config.UploadRule{uploadRule(t)})
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	if got, want := bundle.Request.Args[4], "./doc.md"; got != want {
		t.Errorf("arg = %q, want %q", got, want)
	}
	if bundle.Request.BundleFormat != BundleFormatTarGz {
		t.Errorf("bundle format = %q, want %q", bundle.Request.BundleFormat, BundleFormatTarGz)
	}
	if bundle.Request.BundleSize <= 0 || len(bundle.Data) == 0 {
		t.Fatal("expected non-empty bundle")
	}

	workDir := t.TempDir()
	if err := Extract(bytes.NewReader(bundle.Data), workDir); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workDir, "doc.md"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("extracted data = %q, want hello", data)
	}
}

func TestBuildRequestDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll("docs/sub", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("docs/sub/doc.md", []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}

	bundle, err := BuildRequest("gws", []string{"drive", "files", "create", "--upload", "./docs"}, []config.UploadRule{uploadRule(t)})
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	workDir := t.TempDir()
	if err := Extract(bytes.NewReader(bundle.Data), workDir); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workDir, "docs", "sub", "doc.md"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("extracted data = %q, want hello", data)
	}
}

func TestBuildRequestAbsoluteFileRewritesArg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}

	bundle, err := BuildRequest("gws", []string{"drive", "files", "create", "--upload=" + path}, []config.UploadRule{uploadRule(t)})
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	if got, want := bundle.Request.Args[3], "--upload=./.cmdproxy-upload/0/doc.md"; got != want {
		t.Errorf("arg = %q, want %q", got, want)
	}
	if len(bundle.Request.Uploads) != 1 {
		t.Fatalf("uploads = %d, want 1", len(bundle.Request.Uploads))
	}
	if got, want := bundle.Request.Uploads[0].StagedPath, "./.cmdproxy-upload/0/doc.md"; got != want {
		t.Errorf("staged path = %q, want %q", got, want)
	}
}

func TestBuildRequestMissingFileCreatesMetadataOnly(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	bundle, err := BuildRequest("gws", []string{"drive", "files", "create", "--upload", "./missing.md"}, []config.UploadRule{uploadRule(t)})
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	if bundle.Request.BundleSize != 0 || len(bundle.Data) != 0 {
		t.Fatal("expected no bundle for missing file")
	}
	if len(bundle.Request.Uploads) != 1 || !bundle.Request.Uploads[0].Missing {
		t.Fatalf("uploads = %#v, want one missing upload", bundle.Request.Uploads)
	}
	if got, want := bundle.Request.Args[4], "./missing.md"; got != want {
		t.Errorf("arg = %q, want %q", got, want)
	}
}

func TestExtractRejectsTraversal(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "../escape.txt", Mode: 0600, Size: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	if err := Extract(bytes.NewReader(buf.Bytes()), t.TempDir()); err == nil {
		t.Fatal("expected traversal error")
	}
}

func uploadRule(t *testing.T) config.UploadRule {
	t.Helper()
	match, err := argmatch.ParseSegments([]string{"drive", "files", "create", "*:*"})
	if err != nil {
		t.Fatal(err)
	}
	return config.UploadRule{Match: match, FileArgs: []string{"--upload", "-u"}}
}
