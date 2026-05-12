package g10k

//go:generate goyacc -o puppetfile_parser.go -p pfyy puppetfile_parser.y

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type pfAttribute struct {
	Name        string
	Value       string
	HasArrow    bool
	IsBareValue bool
}

type pfParserState struct {
	rt               *Runtime
	err              error
	pfPath           string
	source           string
	branch           string
	forceForgeVers   bool
	currentModuleDir string
	moduleDirs       []string
	puppetFile       Puppetfile
}

func (rt *Runtime) newPFParserState(pfPath string, source string, branch string, forceForgeVersions bool) *pfParserState {
	moduleDir := "modules"
	if len(rt.ModuleDir) != 0 {
		moduleDir = rt.ModuleDir
	}
	return &pfParserState{
		rt:               rt,
		pfPath:           pfPath,
		source:           source,
		branch:           branch,
		forceForgeVers:   forceForgeVersions,
		currentModuleDir: moduleDir,
		puppetFile: Puppetfile{
			forgeModules: map[string]ForgeModule{},
			gitModules:   map[string]GitModule{},
		},
	}
}

func (s *pfParserState) handleModuledir(value string) {
	if len(s.rt.ModuleDir) != 0 {
		return
	}
	s.currentModuleDir = normalizeDir(value)
	s.moduleDirs = append(s.moduleDirs, s.currentModuleDir)
}

func (s *pfParserState) handleForgeBaseURL(value string) {
	s.puppetFile.forgeBaseURL = value
}

func (s *pfParserState) handleForgeCacheTTL(value string, line string) error {
	ttl, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("error: Can not convert value %s of parameter forge.cacheTtl %s to a golang Duration. valid time units are 300ms, 1.5h or 2h45m. In %s line: %s", value, value, s.pfPath, line)
	}
	s.puppetFile.forgeCacheTTL = ttl
	return nil
}

func (s *pfParserState) handleMod(moduleName string, attrs []pfAttribute, line string) error {
	moduleName = strings.TrimSpace(moduleName)
	if strings.Contains(moduleName, "/") || strings.Contains(moduleName, "-") {
		return s.handleForgeModule(moduleName, attrs, line)
	}
	return s.handleGitModule(moduleName, attrs, line)
}

func (s *pfParserState) handleForgeModule(moduleName string, attrs []pfAttribute, line string) error {
	comp := strings.Split(moduleName, "/")
	if len(comp) != 2 {
		comp = strings.Split(moduleName, "-")
		if len(comp) != 2 {
			return fmt.Errorf("Error: Forge module name is invalid! Should be like puppetlabs/apt or puppetlabs-apt, but is: %s in %s line: %s", moduleName, s.pfPath, line)
		}
	}
	author := comp[0]
	name := comp[1]

	if _, ok := s.puppetFile.forgeModules[name]; ok {
		return fmt.Errorf("Error: Duplicate forge module found in %s for module %s/%s line: %s", s.pfPath, author, name, line)
	}

	version := "present"
	sha256sum := ""
	isForgeNotationGit := false
	forgeGitURL := ""

	for _, a := range attrs {
		if a.IsBareValue {
			version = a.Value
			continue
		}
		if !a.HasArrow {
			if a.Name != "" {
				version = a.Name
			}
			continue
		}
		if a.Name == "sha256sum" {
			sha256sum = a.Value
			continue
		}
		if a.Name == "git" {
			isForgeNotationGit = true
			forgeGitURL = a.Value
			continue
		}
		version = a.Value
	}

	// Normalize version: if it starts with ':' (from a quoted symbol like ':latest'),
	// treat it as a symbol and strip the leading colon
	if strings.HasPrefix(version, ":") && !strings.Contains(version, ".") {
		version = version[1:]
	}

	if isForgeNotationGit {
		if _, ok := s.puppetFile.forgeModules[name]; ok {
			return fmt.Errorf("Error: Forge Puppet module with same name found in %s for module %s line: %s", s.pfPath, name, line)
		}
		if _, ok := s.puppetFile.gitModules[name]; ok {
			return fmt.Errorf("Error: Duplicate module found in %s for module %s line: %s", s.pfPath, name, line)
		}
		gm, _, err := s.parseGitAttributes(name, attrs, line)
		if err != nil {
			return err
		}
		if len(gm.git) == 0 {
			gm.git = forgeGitURL
		}
		gm.moduleDir = s.currentModuleDir
		if s.rt.Config.IgnoreUnreachableModules {
			s.rt.Debugf("Setting :ignore_unreachable for Git module " + name)
			gm.ignoreUnreachable = true
		}
		s.puppetFile.gitModules[name] = gm
		return nil
	}

	if s.forceForgeVers && (version == "present" || version == "latest") {
		return fmt.Errorf("Error: Found %s setting for forge module in %s for module %s/%s line: %s and force_forge_versions is set to true! Please specify a version (e.g. '2.3.0')", version, s.pfPath, author, name, line)
	}
	if _, ok := s.puppetFile.gitModules[name]; ok {
		return fmt.Errorf("Error: Forge Puppet module with same name found in %s for module %s line: %s", s.pfPath, name, line)
	}

	if len(s.puppetFile.forgeBaseURL) == 0 {
		s.puppetFile.forgeBaseURL = s.rt.Config.ForgeBaseURL
	}
	s.puppetFile.forgeModules[name] = ForgeModule{
		version:      version,
		name:         name,
		author:       author,
		sha256sum:    sha256sum,
		moduleDir:    s.currentModuleDir,
		sourceBranch: s.source + "_" + s.branch,
	}
	return nil
}

func (s *pfParserState) handleGitModule(moduleName string, attrs []pfAttribute, line string) error {
	if strings.Contains(moduleName, "-") {
		s.rt.Warnf("Warning: Found invalid character '-' in Puppet module name " + moduleName + " in " + s.pfPath + " line: " + line + "\n See module guidelines: https://docs.puppet.com/puppet/latest/reference/lang_reserved.html#modules")
	}
	if len(attrs) == 0 {
		if s.rt.DryRun {
			return fmt.Errorf("Error: Missing :git url in %s for module %s line: %s", s.pfPath, moduleName, line)
		}
		return nil
	}
	if len(attrs) > 4 {
		return fmt.Errorf("Error: Too many attributes in %s for module %s line: %s", s.pfPath, moduleName, line)
	}
	if _, ok := s.puppetFile.gitModules[moduleName]; ok {
		return fmt.Errorf("Error: Duplicate module found in %s for module %s line: %s", s.pfPath, moduleName, line)
	}

	gm, hasGitOrLocalAttr, err := s.parseGitAttributes(moduleName, attrs, line)
	if err != nil {
		return err
	}

	if len(gm.git) == 0 && !hasGitOrLocalAttr {
		return fmt.Errorf("Error: Missing :git url in %s for module %s line: %s", s.pfPath, moduleName, line)
	}

	if _, ok := s.puppetFile.forgeModules[moduleName]; ok {
		return fmt.Errorf("error: git puppet module with same name found in %s for module %s line: %s", s.pfPath, moduleName, line)
	}
	if s.rt.Config.IgnoreUnreachableModules {
		s.rt.Debugf("Setting :ignore_unreachable for Git module " + moduleName)
		gm.ignoreUnreachable = true
	}
	s.puppetFile.gitModules[moduleName] = gm
	return nil
}

func (s *pfParserState) parseGitAttributes(moduleName string, attrs []pfAttribute, line string) (GitModule, bool, error) {
	gm := GitModule{moduleDir: s.currentModuleDir}
	seenConflicting := make([]string, 0, 3)
	seenKeys := make(map[string]bool)
	hasGitOrLocalAttr := false

	for _, a := range attrs {
		if !a.HasArrow {
			return GitModule{}, false, fmt.Errorf("Error: Trailing comma or invalid setting for module found in %s for module %s line: %s", s.pfPath, moduleName, line)
		}
		if seenKeys[a.Name] {
			return GitModule{}, false, fmt.Errorf("Error: Duplicate module found in %s for module %s line: %s", s.pfPath, moduleName, line)
		}
		seenKeys[a.Name] = true

		switch a.Name {
		case "git":
			hasGitOrLocalAttr = true
			if strings.Contains(a.Value, "ProxyCommand") {
				return GitModule{}, false, fmt.Errorf("Error: Found ProxyCommand option in git url in %s for module %s line: %s", s.pfPath, moduleName, line)
			}
			gm.git = a.Value
		case "branch":
			seenConflicting = append(seenConflicting, ":branch")
			if a.Value == ":control_branch" || a.Value == "control_branch" {
				gm.link = true
			} else {
				gm.branch = a.Value
			}
		case "tag":
			seenConflicting = append(seenConflicting, ":tag")
			gm.tag = a.Value
		case "commit":
			seenConflicting = append(seenConflicting, ":commit")
			gm.commit = a.Value
		case "ref":
			seenConflicting = append(seenConflicting, ":ref")
			gm.ref = a.Value
		case "install_path":
			gm.installPath = a.Value
		case "link":
			seenConflicting = append(seenConflicting, ":link")
			link, err := strconv.ParseBool(a.Value)
			if err != nil {
				return GitModule{}, false, fmt.Errorf("Error: Can not convert value %s of parameter link to boolean. In %s for module %s line: %s", a.Value, s.pfPath, moduleName, line)
			}
			gm.link = link
		case "ignore-unreachable", "ignore_unreachable":
			ignoreUnreachable, err := strconv.ParseBool(a.Value)
			if err != nil {
				return GitModule{}, false, fmt.Errorf("Error: Can not convert value %s of parameter %s to boolean. In %s for module %s line: %s", a.Value, a.Name, s.pfPath, moduleName, line)
			}
			gm.ignoreUnreachable = ignoreUnreachable
		case "fallback", "default_branch":
			gm.fallback = make([]string, strings.Count(a.Value, "|")+1)
			for i, fallbackBranch := range strings.Split(a.Value, "|") {
				gm.fallback[i] = strings.TrimSpace(fallbackBranch)
			}
		case "local":
			hasGitOrLocalAttr = true
			local, err := strconv.ParseBool(a.Value)
			if err != nil {
				return GitModule{}, false, fmt.Errorf("Error: Can not convert value %s of parameter local to boolean. In %s for module %s line: %s", a.Value, s.pfPath, moduleName, line)
			}
			gm.local = local
		case "use_ssh_agent":
			useSSHAgent, err := strconv.ParseBool(a.Value)
			if err != nil {
				return GitModule{}, false, fmt.Errorf("Error: Can not convert value %s of parameter use_ssh_agent to boolean. In %s for module %s line: %s", a.Value, s.pfPath, moduleName, line)
			}
			gm.useSSHAgent = useSSHAgent
		default:
			return GitModule{}, false, fmt.Errorf("Error: Trailing comma or invalid setting for module found in %s for module %s line: %s", s.pfPath, moduleName, line)
		}
	}

	if len(seenConflicting) > 1 {
		return GitModule{}, false, fmt.Errorf("error: found conflicting git attributes %s, in %s for module %s line: %s", strings.Join(seenConflicting, ", "), s.pfPath, moduleName, line)
	}

	return gm, hasGitOrLocalAttr, nil
}

func (s *pfParserState) finalize(sshKey string) Puppetfile {
	s.puppetFile.privateKey = sshKey
	s.puppetFile.source = s.source
	s.puppetFile.sourceBranch = s.branch
	if len(s.moduleDirs) == 0 {
		s.moduleDirs = append(s.moduleDirs, s.currentModuleDir)
	}
	s.puppetFile.moduleDirs = s.moduleDirs
	if len(s.puppetFile.forgeBaseURL) == 0 {
		s.puppetFile.forgeBaseURL = s.rt.Config.ForgeBaseURL
	}
	return s.puppetFile
}

type pfLexer struct {
	lines       []string
	lineIdx     int
	col         int
	currentLine string
	logicalLine string
	pfPath      string
	state       *pfParserState
}

func newPFLexer(input string, pfPath string, state *pfParserState) *pfLexer {
	return &pfLexer{lines: strings.Split(input, "\n"), pfPath: pfPath, state: state}
}

func (l *pfLexer) Error(_ string) {
	if len(strings.TrimSpace(l.currentLine)) == 0 {
		l.state.err = fmt.Errorf("Error: Could not interpret Puppetfile %s", l.pfPath)
	} else if l.state.rt.DryRun {
		l.state.err = fmt.Errorf("Error: Could not interpret line: %s In %s", l.currentLine, l.pfPath)
	} else {
		l.state.err = fmt.Errorf("Error: Trailing comma or invalid setting for module found in %s line: %s", l.pfPath, l.currentLine)
	}
}

func (l *pfLexer) Lex(lval *pfyySymType) int {
	for {
		if l.lineIdx >= len(l.lines) {
			return 0
		}
		line := l.lines[l.lineIdx]
		l.currentLine = line
		if l.col >= len(line) {
			effectiveLine := strings.TrimSpace(stripPuppetComment(line))
			if len(effectiveLine) > 0 {
				l.logicalLine += effectiveLine
			}
			l.lineIdx++
			l.col = 0
			if len(effectiveLine) == 0 {
				continue
			}
			if strings.HasSuffix(effectiveLine, ",") {
				continue
			}
			if len(l.logicalLine) > 0 {
				l.currentLine = l.logicalLine
				l.logicalLine = ""
			}
			return NEWLINE
		}

		for l.col < len(line) && (line[l.col] == ' ' || line[l.col] == '\t' || line[l.col] == '\r') {
			l.col++
		}
		if l.col >= len(line) {
			continue
		}
		if line[l.col] == '#' {
			l.col = len(line)
			continue
		}

		if line[l.col] == ',' {
			l.col++
			return COMMA
		}
		if l.col+1 < len(line) && line[l.col] == '=' && line[l.col+1] == '>' {
			l.col += 2
			return ARROW
		}
		if line[l.col] == '\'' || line[l.col] == '"' {
			q := line[l.col]
			l.col++
			start := l.col
			for l.col < len(line) && line[l.col] != q {
				l.col++
			}
			if l.col >= len(line) {
				lval.str = line[start:]
				return STRING
			}
			lval.str = line[start:l.col]
			l.col++
			return STRING
		}
		if line[l.col] == ':' {
			l.col++
			start := l.col
			for l.col < len(line) && isSymbolChar(line[l.col]) {
				l.col++
			}
			lval.str = line[start:l.col]
			return SYMBOL
		}

		start := l.col
		for l.col < len(line) && !isDelimiter(line[l.col]) {
			l.col++
		}
		word := line[start:l.col]
		switch word {
		case "mod":
			return MOD
		case "moduledir":
			return MODULEDIR
		case "forge.baseUrl", "forge.baseURL":
			return FORGEBASEURL
		case "forge.cacheTtl", "forge.cacheTTL":
			return FORGECACHETTL
		default:
			lval.str = word
			return WORD
		}
	}
}

func stripPuppetComment(line string) string {
	if strings.Contains(line, "#") {
		return strings.Split(line, "#")[0]
	}
	return line
}

func isDelimiter(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == ',' || c == '\n'
}

func isSymbolChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-'
}

// ParsePuppetfile parses a Puppetfile using the runtime's module settings.
func (rt *Runtime) ParsePuppetfile(pfPath string, sshKey string, source string, branch string, forceForgeVersions bool) (Puppetfile, error) {
	rawContent, err := os.ReadFile(pfPath)
	if err != nil {
		return Puppetfile{}, fmt.Errorf("ParsePuppetfile(): Error while opening Puppetfile %s Error: %w", pfPath, err)
	}
	content := string(rawContent)
	state := rt.newPFParserState(pfPath, source, branch, forceForgeVersions)
	lexer := newPFLexer(content, pfPath, state)
	if pfyyParse(lexer) != 0 && state.err == nil {
		lexer.Error("syntax error")
	}
	if state.err != nil {
		return Puppetfile{}, state.err
	}
	return state.finalize(sshKey), nil
}
