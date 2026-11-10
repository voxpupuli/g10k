package puppetfileparser_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/voxpupuli/g10k/pkg/g10k/puppetfileparser"
)

type event struct {
	kind  string
	value string
	attrs []puppetfileparser.Attribute
	line  string
}

type recordingHandler struct {
	events []event
	err    error
}

func (h *recordingHandler) HandleModuleDir(value string) error {
	h.events = append(h.events, event{kind: "moduledir", value: value})
	return h.err
}

func (h *recordingHandler) HandleForgeBaseURL(value string) error {
	h.events = append(h.events, event{kind: "forge.baseUrl", value: value})
	return h.err
}

func (h *recordingHandler) HandleForgeCacheTTL(value, line string) error {
	h.events = append(h.events, event{kind: "forge.cacheTtl", value: value, line: line})
	return h.err
}

func (h *recordingHandler) HandleModule(name string, attrs []puppetfileparser.Attribute, line string) error {
	h.events = append(h.events, event{kind: "mod", value: name, attrs: attrs, line: line})
	return h.err
}

func checkEvents(t *testing.T, input string, want []event) {
	t.Helper()
	h := &recordingHandler{}
	if err := puppetfileparser.Parse(input, h); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(h.events, want) {
		t.Fatalf("events = %#v, want %#v", h.events, want)
	}
}

func TestParseOrdering(t *testing.T) {
	checkEvents(t, "moduledir 'first'\nmod 'a/b'\nmoduledir 'second'\nmod 'c/d'\n", []event{
		{kind: "moduledir", value: "first"},
		{kind: "mod", value: "a/b", line: "mod 'a/b'"},
		{kind: "moduledir", value: "second"},
		{kind: "mod", value: "c/d", line: "mod 'c/d'"},
	})
}

func TestParseHandlerErrorStopsParsing(t *testing.T) {
	for _, input := range []string{
		"moduledir 'first'\nmod 'later'\n",
		"forge.baseUrl 'first'\nmod 'later'\n",
		"forge.cacheTtl 'bad'\nmod 'later'\n",
		"mod 'first'\nmod 'later'\n",
	} {
		t.Run(input, func(t *testing.T) {
			want := errors.New("handler error")
			h := &recordingHandler{err: want}
			if err := puppetfileparser.Parse(input, h); !errors.Is(err, want) {
				t.Fatalf("error = %v, want %v", err, want)
			}
			if len(h.events) != 1 {
				t.Fatalf("events = %#v, want exactly one", h.events)
			}
		})
	}
}

func TestParseSyntaxError(t *testing.T) {
	h := &recordingHandler{}
	err := puppetfileparser.Parse("moduledir 'ok'\nmod 'broken', :git =>\n", h)
	var syntaxErr *puppetfileparser.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("error = %v, want SyntaxError", err)
	}
	if syntaxErr.Line != "mod 'broken', :git =>" {
		t.Fatalf("line = %q", syntaxErr.Line)
	}
	if len(h.events) != 1 || h.events[0].value != "ok" {
		t.Fatalf("events before error = %#v", h.events)
	}
}

func TestParseNilHandler(t *testing.T) {
	if err := puppetfileparser.Parse("", nil); err == nil {
		t.Fatal("expected an error for a nil handler")
	}
}

func TestParseEmptyAndComments(t *testing.T) {
	checkEvents(t, "\r\n# comment\n  # comment\r\n", nil)
}
