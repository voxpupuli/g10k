package g10k

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/voxpupuli/g10k/pkg/g10k/puppetfileparser"
)

var tarballNamePattern = regexp.MustCompile(`^(?:[A-Za-z0-9_]+[-/])?[A-Za-z0-9_]+$`)
var githubRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func (s *pfParserState) handleTarballModule(declaration puppetfileparser.Tarball, line string) error {
	parts := strings.FieldsFunc(declaration.Name, func(r rune) bool { return r == '/' || r == '-' })
	if !tarballNamePattern.MatchString(declaration.Name) {
		return fmt.Errorf("invalid tarball module name %q in %s line: %s", declaration.Name, s.pfPath, line)
	}
	name := parts[len(parts)-1]
	// TODO: Centralize duplicate checks across module types while preserving legacy diagnostics.
	if _, ok := s.puppetFile.forgeModules[name]; ok {
		return fmt.Errorf("duplicate module %q in %s line: %s", name, s.pfPath, line)
	}
	if _, ok := s.puppetFile.gitModules[name]; ok {
		return fmt.Errorf("duplicate module %q in %s line: %s", name, s.pfPath, line)
	}
	if _, ok := s.puppetFile.tarballModules[name]; ok {
		return fmt.Errorf("duplicate module %q in %s line: %s", name, s.pfPath, line)
	}
	if declaration.GitHubRepository != "" {
		repositoryParts := strings.Split(declaration.GitHubRepository, "/")
		if !githubRepositoryPattern.MatchString(declaration.GitHubRepository) ||
			repositoryParts[0] == "." || repositoryParts[0] == ".." ||
			repositoryParts[1] == "." || repositoryParts[1] == ".." {
			return fmt.Errorf("invalid github_tarball repository %q in %s line: %s", declaration.GitHubRepository, s.pfPath, line)
		}
		if declaration.Version == "." || declaration.Version == ".." {
			return fmt.Errorf("invalid github_tarball version %q in %s line: %s", declaration.Version, s.pfPath, line)
		}
	}
	module := TarballModule{
		name: name, source: declaration.Source, githubRepo: declaration.GitHubRepository,
		moduleDir: s.currentModuleDir, stripComponents: declaration.StripComponents,
	}
	if module.githubRepo == "" {
		module.checksum = declaration.Version
	} else {
		module.version = declaration.Version
		module.stripComponents = 1
	}
	if s.puppetFile.tarballModules == nil {
		s.puppetFile.tarballModules = make(map[string]TarballModule)
	}
	s.puppetFile.tarballModules[name] = module
	return nil
}
