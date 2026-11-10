package puppetfileparser

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Tarball describes either a direct archive or a GitHub repository archive.
// For direct archives, Version is an optional SHA-256 digest (from version or
// checksum); for GitHub archives it is the requested version, not a digest.
type Tarball struct {
	Name             string
	Source           string
	Version          string
	GitHubRepository string
}

// ParseTarball interprets module attributes as a tarball declaration.
// It returns false for other module types. It does not resolve GitHub versions,
// fetch archives, or interpret paths within archives.
func ParseTarball(name string, attrs []Attribute) (Tarball, bool, error) {
	direct, github := false, false
	for _, attr := range attrs {
		if keyword, ok := attachedKeyword(attr); ok && keyword.Name == "type" && strings.TrimPrefix(keyword.Value, ":") == "tarball" {
			direct = true
		}
		if attr.HasArrow || attr.IsKeyword {
			if attr.Name == "type" && strings.TrimPrefix(attr.Value, ":") == "tarball" {
				direct = true
			}
			if attr.Name == "github_tarball" {
				github = true
			}
		}
	}
	if !direct && !github {
		return Tarball{}, false, nil
	}
	if direct && github {
		return Tarball{}, true, fmt.Errorf("module %q: conflicting tarball types", name)
	}

	result := Tarball{Name: name}
	seen := make(map[string]bool)
	for _, attr := range attrs {
		if direct {
			if keyword, ok := attachedKeyword(attr); ok {
				attr = keyword
			}
		}
		key := attr.Name
		if attr.IsBareValue {
			if direct {
				return Tarball{}, true, fmt.Errorf("module %q: invalid bare tarball version", name)
			}
			key = "version"
		} else if !attr.HasArrow && !attr.IsKeyword {
			return Tarball{}, true, fmt.Errorf("module %q: invalid tarball attribute %q", name, key)
		}
		if direct && key == "checksum" {
			key = "version"
		}
		if seen[key] {
			return Tarball{}, true, fmt.Errorf("module %q: duplicate tarball attribute %q", name, key)
		}
		seen[key] = true
		switch {
		case direct && key == "type":
			if strings.TrimPrefix(attr.Value, ":") != "tarball" {
				return Tarball{}, true, fmt.Errorf("module %q: invalid tarball type %q", name, attr.Value)
			}
		case direct && key == "source":
			result.Source = attr.Value
		case key == "version":
			result.Version = attr.Value
		case github && key == "github_tarball":
			result.GitHubRepository = attr.Value
		default:
			return Tarball{}, true, fmt.Errorf("module %q: invalid tarball attribute %q", name, key)
		}
	}

	if direct {
		if strings.TrimSpace(result.Source) == "" {
			return Tarball{}, true, fmt.Errorf("module %q: tarball source is required", name)
		}
		if seen["version"] {
			digest, err := hex.DecodeString(result.Version)
			if err != nil || len(digest) != 32 {
				return Tarball{}, true, fmt.Errorf("module %q: tarball version must be a SHA-256 digest", name)
			}
		}
	} else if strings.TrimSpace(result.GitHubRepository) == "" {
		return Tarball{}, true, fmt.Errorf("module %q: github_tarball repository is required", name)
	}
	return result, true, nil
}

func attachedKeyword(attr Attribute) (Attribute, bool) {
	if !attr.IsBareValue {
		return Attribute{}, false
	}
	name, value, ok := strings.Cut(attr.Value, ":")
	if !ok || name == "" || value == "" {
		return Attribute{}, false
	}
	if value[0] == '\'' || value[0] == '"' {
		if len(value) < 2 || value[len(value)-1] != value[0] {
			return Attribute{}, false
		}
		value = value[1 : len(value)-1]
	}
	return Attribute{Name: name, Value: value, IsKeyword: true}, true
}
