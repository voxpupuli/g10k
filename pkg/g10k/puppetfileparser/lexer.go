package puppetfileparser

//go:generate go tool goyacc -o parser_generated.go -v "" -p pfyy parser.y

import "strings"

type pfLexer struct {
	lines       []string
	lineIdx     int
	col         int
	currentLine string
	logicalLine string
	handler     Handler
	err         error
}

func newPFLexer(input string, handler Handler) *pfLexer {
	return &pfLexer{lines: strings.Split(input, "\n"), handler: handler}
}

func (l *pfLexer) Error(_ string) {
	l.err = &SyntaxError{Line: l.currentLine}
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
