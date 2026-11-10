// Package puppetfileparser parses the Puppetfile syntax supported by g10k.
// It invokes a handler in declaration order and does not resolve or deploy modules.
package puppetfileparser

import "fmt"

// Attribute is a module argument. A bare value has IsBareValue set; a symbol
// without an arrow has Name set and HasArrow false.
type Attribute struct {
	Name        string
	Value       string
	HasArrow    bool
	IsBareValue bool
}

// Handler receives declarations as they are parsed. Line is the logical
// declaration text used for diagnostics, including all continued lines.
// An error returned by a handler stops parsing and is returned unchanged.
type Handler interface {
	HandleModuleDir(value string) error
	HandleForgeBaseURL(value string) error
	HandleForgeCacheTTL(value, line string) error
	HandleModule(name string, attributes []Attribute, line string) error
}

// SyntaxError reports a declaration that the parser could not interpret.
type SyntaxError struct {
	Line string
}

func (e *SyntaxError) Error() string {
	if e.Line == "" {
		return "could not interpret Puppetfile"
	}
	return fmt.Sprintf("could not interpret Puppetfile line: %s", e.Line)
}

// Parse parses input and calls handler in declaration order. Calls made before
// an error are not rolled back; the caller must discard partial results if needed.
func Parse(input string, handler Handler) error {
	if handler == nil {
		return fmt.Errorf("puppetfileparser: nil handler")
	}
	lexer := newPFLexer(input, handler)
	if pfyyParse(lexer) != 0 && lexer.err == nil {
		lexer.Error("syntax error")
	}
	return lexer.err
}
