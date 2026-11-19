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

func (rt *Runtime) extractTarball(archive, target string, stripComponents int) (resultErr error) {
	if stripComponents < 0 {
		return fmt.Errorf("strip_components must be a non-negative integer")
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer joinTarballCloseError(&resultErr, f, "close tarball archive")
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer joinTarballCloseError(&resultErr, gz, "close tarball gzip reader")
	root, err := os.OpenRoot(target)
	if err != nil {
		return err
	}
	defer joinTarballCloseError(&resultErr, root, "close tarball extraction root")
	reader := tar.NewReader(gz)
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
		if stripComponents > 0 {
			name, err = stripTarballEntryName(h.Name, stripComponents)
			if err != nil {
				return err
			}
			if name == "" {
				continue
			}
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
		case tar.TypeReg:
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
			if stripComponents > 0 {
				link, err = stripTarballEntryName(h.Linkname, stripComponents)
				if err != nil {
					return err
				}
				if link == "" {
					continue
				}
			}
			if err := root.Link(filepath.FromSlash(link), localName); err != nil {
				return fmt.Errorf("create hardlink %q: %w", name, err)
			}
		default:
			return fmt.Errorf("unsupported tar entry type %d for %q", h.Typeflag, name)
		}
	}
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return fmt.Errorf("finish gzip stream: %w", err)
	}
	return nil
}

func validateGitHubTarballRoot(archive string) (resultErr error) {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer joinTarballCloseError(&resultErr, f, "close tarball archive")
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer joinTarballCloseError(&resultErr, gz, "close tarball gzip reader")
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
		parts := strings.SplitN(name, "/", 2)
		if top == "" {
			top = parts[0]
		}
		if parts[0] != top {
			return fmt.Errorf("GitHub archive contains multiple root directories")
		}
		if len(parts) == 1 && h.Typeflag != tar.TypeDir {
			return fmt.Errorf("GitHub archive has a non-directory root %q", h.Name)
		}
		if h.Typeflag == tar.TypeLink {
			link, err := tarballEntryName(h.Linkname)
			if err != nil {
				return err
			}
			if !strings.HasPrefix(link, top+"/") {
				return fmt.Errorf("invalid GitHub archive hardlink %q", h.Linkname)
			}
		}
	}
	if top == "" {
		return fmt.Errorf("empty GitHub archive")
	}
	return nil
}

func stripTarballEntryName(name string, count int) (string, error) {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '/' })
	if len(parts) <= count {
		return "", nil
	}
	return tarballEntryName(strings.Join(parts[count:], "/"))
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
