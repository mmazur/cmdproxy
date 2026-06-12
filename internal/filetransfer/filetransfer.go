package filetransfer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mmazur/cmdproxy/internal/config"
	"github.com/mmazur/cmdproxy/internal/policy/argmatch"
	"github.com/mmazur/cmdproxy/internal/protocol"
)

const (
	BundleFormatTarGz = "tar.gz"
	UploadRoot        = ".cmdproxy-upload"

	maxExtractedFiles = 10000
	maxExtractedBytes = 1 << 30
)

type Bundle struct {
	Request protocol.Request
	Data    []byte
}

func BuildRequest(cmd string, args []string, rules []config.UploadRule) (Bundle, error) {
	req := protocol.Request{Cmd: cmd, Args: append([]string(nil), args...)}

	rule, ok := matchingUploadRule(args, rules)
	if !ok {
		return Bundle{Request: req}, nil
	}

	paths := findFileArgs(req.Args, rule.FileArgs)
	if len(paths) == 0 {
		return Bundle{Request: req}, nil
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	wroteFile := false

	for uploadIndex, pathArg := range paths {
		original := pathArg.Value
		staged := stagedPath(original, uploadIndex)
		if staged != original {
			if pathArg.Inline {
				req.Args[pathArg.ArgIndex] = pathArg.Flag + "=" + staged
			} else {
				req.Args[pathArg.ArgIndex] = staged
			}
		}

		meta := protocol.UploadMeta{
			ArgIndex:     pathArg.ArgIndex,
			OriginalPath: original,
			StagedPath:   staged,
		}

		info, err := os.Lstat(original)
		if err != nil {
			if os.IsNotExist(err) {
				meta.Missing = true
				req.Uploads = append(req.Uploads, meta)
				continue
			}
			return Bundle{}, fmt.Errorf("stat upload path %s: %w", original, err)
		}

		if err := addPath(tw, original, staged, info); err != nil {
			return Bundle{}, err
		}
		wroteFile = true
		req.Uploads = append(req.Uploads, meta)
	}

	if err := tw.Close(); err != nil {
		return Bundle{}, fmt.Errorf("tar close: %w", err)
	}
	if err := gz.Close(); err != nil {
		return Bundle{}, fmt.Errorf("gzip close: %w", err)
	}

	if wroteFile {
		req.BundleFormat = BundleFormatTarGz
		req.BundleSize = int64(buf.Len())
		return Bundle{Request: req, Data: buf.Bytes()}, nil
	}
	return Bundle{Request: req}, nil
}

func Extract(r io.Reader, workDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var files int
	var bytesWritten int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		files++
		if files > maxExtractedFiles {
			return fmt.Errorf("too many files in upload bundle")
		}
		if hdr.Size < 0 || bytesWritten+hdr.Size > maxExtractedBytes {
			return fmt.Errorf("upload bundle exceeds extracted size limit")
		}

		dest, err := safeExtractPath(workDir, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0700); err != nil {
				return fmt.Errorf("mkdir %s: %w", dest, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(dest), err)
			}
			mode := hdr.FileInfo().Mode().Perm()
			if mode&0111 == 0 {
				mode = 0600
			} else {
				mode = 0700
			}
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return fmt.Errorf("create %s: %w", dest, err)
			}
			n, copyErr := io.Copy(f, tr)
			closeErr := f.Close()
			if copyErr != nil {
				return fmt.Errorf("write %s: %w", dest, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close %s: %w", dest, closeErr)
			}
			bytesWritten += n
		default:
			return fmt.Errorf("unsupported tar entry type %d for %s", hdr.Typeflag, hdr.Name)
		}
	}
}

func matchingUploadRule(args []string, rules []config.UploadRule) (config.UploadRule, bool) {
	for _, rule := range rules {
		if argmatch.Match(rule.Match, args) {
			return rule, true
		}
	}
	return config.UploadRule{}, false
}

type fileArg struct {
	Flag     string
	Value    string
	ArgIndex int
	Inline   bool
}

func findFileArgs(args []string, flags []string) []fileArg {
	var found []fileArg
	for i, arg := range args {
		for _, flag := range flags {
			if arg == flag {
				if i+1 < len(args) {
					found = append(found, fileArg{
						Flag:     flag,
						Value:    args[i+1],
						ArgIndex: i + 1,
					})
				}
				continue
			}
			if strings.HasPrefix(arg, flag+"=") {
				found = append(found, fileArg{
					Flag:     flag,
					Value:    strings.TrimPrefix(arg, flag+"="),
					ArgIndex: i,
					Inline:   true,
				})
			}
		}
	}
	return found
}

func stagedPath(path string, index int) string {
	if safeRelativePath(path) {
		return path
	}
	base := filepath.Base(filepath.Clean(path))
	if base == "." || base == string(filepath.Separator) {
		base = "upload"
	}
	return "./" + filepath.ToSlash(filepath.Join(UploadRoot, fmt.Sprintf("%d", index), base))
}

func safeRelativePath(path string) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	return clean != "." && !hasParentDirElement(path)
}

func addPath(tw *tar.Writer, src, staged string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("upload path %s is a symlink; symlinks are not supported", src)
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return fmt.Errorf("upload path %s is not a regular file or directory", src)
	}
	if info.IsDir() {
		return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			entryInfo, err := entry.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			name := staged
			if rel != "." {
				name = filepath.Join(staged, rel)
			}
			return addEntry(tw, path, filepath.ToSlash(name), entryInfo)
		})
	}
	return addEntry(tw, src, filepath.ToSlash(staged), info)
}

func addEntry(tw *tar.Writer, src, name string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("upload path %s is a symlink; symlinks are not supported", src)
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return fmt.Errorf("upload path %s is not a regular file or directory", src)
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("tar header %s: %w", src, err)
	}
	hdr.Name = name
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("tar write header %s: %w", src, err)
	}
	if info.IsDir() {
		return nil
	}
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open upload path %s: %w", src, err)
	}
	defer f.Close()
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("tar write %s: %w", src, err)
	}
	return nil
}

func safeExtractPath(workDir, name string) (string, error) {
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) || clean == "." || hasParentDirElement(name) {
		return "", fmt.Errorf("unsafe tar path: %s", name)
	}
	dest := filepath.Join(workDir, clean)
	rel, err := filepath.Rel(workDir, dest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("tar path escapes workdir: %s", name)
	}
	return dest, nil
}

func hasParentDirElement(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
