package g10k

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func (rt *Runtime) extractTarball(archive, target string, stripRoot bool) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	root, err := os.OpenRoot(target)
	if err != nil {
		return err
	}
	defer root.Close()
	reader := tar.NewReader(gz)
	top := ""
	for {
		h, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name, err := tarballEntryName(h.Name)
		if err != nil {
			return err
		}
		if stripRoot {
			parts := strings.SplitN(name, "/", 2)
			if top == "" {
				top = parts[0]
			}
			if parts[0] != top {
				return fmt.Errorf("GitHub archive contains multiple root directories")
			}
			if len(parts) == 1 {
				if h.Typeflag != tar.TypeDir {
					return fmt.Errorf("GitHub archive has a non-directory root %q", h.Name)
				}
				continue
			}
			name = parts[1]
		}
		if name == "." {
			if h.Typeflag != tar.TypeDir {
				return fmt.Errorf("invalid root archive entry %q", h.Name)
			}
			continue
		}
		if rt.matchSkiplistContent(name) {
			continue
		}
		if err := root.MkdirAll(filepath.FromSlash(path.Dir(name)), 0o755); err != nil {
			return fmt.Errorf("create parent for %q: %w", name, err)
		}
		localName := filepath.FromSlash(name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(localName, 0o755); err != nil {
				return fmt.Errorf("create directory %q: %w", name, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			file, err := root.OpenFile(localName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(h.Mode)&0o777)
			if err != nil {
				return fmt.Errorf("create file %q: %w", name, err)
			}
			_, copyErr := io.Copy(file, reader)
			closeErr := file.Close()
			if copyErr != nil {
				return fmt.Errorf("extract file %q: %w", name, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close file %q: %w", name, closeErr)
			}
		case tar.TypeSymlink:
			if h.Linkname == "" || strings.Contains(h.Linkname, "\\") || path.IsAbs(h.Linkname) ||
				!filepath.IsLocal(filepath.FromSlash(path.Join(path.Dir(name), h.Linkname))) {
				return fmt.Errorf("unsafe symlink %q -> %q", name, h.Linkname)
			}
			if err := root.Symlink(filepath.FromSlash(h.Linkname), localName); err != nil {
				return fmt.Errorf("create symlink %q: %w", name, err)
			}
		case tar.TypeLink:
			link, err := tarballEntryName(h.Linkname)
			if err != nil {
				return err
			}
			if stripRoot {
				prefix := top + "/"
				if !strings.HasPrefix(link, prefix) {
					return fmt.Errorf("invalid GitHub archive hardlink %q", h.Linkname)
				}
				link = strings.TrimPrefix(link, prefix)
			}
			if err := root.Link(filepath.FromSlash(link), localName); err != nil {
				return fmt.Errorf("create hardlink %q: %w", name, err)
			}
		default:
			return fmt.Errorf("unsupported tar entry type %d for %q", h.Typeflag, name)
		}
	}
	if stripRoot && top == "" {
		return fmt.Errorf("empty GitHub archive")
	}
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return fmt.Errorf("finish gzip stream: %w", err)
	}
	return nil
}

func tarballEntryName(name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || path.IsAbs(name) {
		return "", fmt.Errorf("unsafe tar entry %q", name)
	}
	clean := path.Clean(name)
	if !filepath.IsLocal(filepath.FromSlash(clean)) {
		return "", fmt.Errorf("unsafe tar entry %q", name)
	}
	return clean, nil
}
