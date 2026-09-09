package g10k

import (
	"os"
	"reflect"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("GIT_CONFIG_COUNT", "2")
	_ = os.Setenv("GIT_CONFIG_KEY_0", "maintenance.auto")
	_ = os.Setenv("GIT_CONFIG_VALUE_0", "false")
	_ = os.Setenv("GIT_CONFIG_KEY_1", "gc.auto")
	_ = os.Setenv("GIT_CONFIG_VALUE_1", "0")
	os.Exit(m.Run())
}

func TestSplitCommandLine(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{name: "hello", input: `hello`, want: []string{"hello"}},
		{name: "words", input: `hello goodbye`, want: []string{"hello", "goodbye"}},
		{name: "repeated whitespace", input: `hello   goodbye`, want: []string{"hello", "goodbye"}},
		{name: "glob characters are literal", input: `glob* test?`, want: []string{"glob*", "test?"}},
		{name: "escaped characters", input: `don\'t you know the dewey decimal system\?`, want: []string{"don't", "you", "know", "the", "dewey", "decimal", "system?"}},
		{name: "quoted apostrophe", input: `'don'\''t you know the dewey decimal system?'`, want: []string{"don't you know the dewey decimal system?"}},
		{name: "empty quoted word", input: `one '' two`, want: []string{"one", "", "two"}},
		{name: "escaped newline", input: "text with\\\na backslash-escaped newline", want: []string{"text", "witha", "backslash-escaped", "newline"}},
		{name: "quoted newline", input: "text \"with\na\" quoted newline", want: []string{"text", "with\na", "quoted", "newline"}},
		{name: "double quote escape rules", input: `"quoted\d\\\" text with\
a backslash-escaped newline"`, want: []string{`quoted\d\" text witha backslash-escaped newline`}},
		{name: "escaped newline in the middle", input: "text with an escaped \\\n newline in the middle", want: []string{"text", "with", "an", "escaped", "newline", "in", "the", "middle"}},
		{name: "adjacent quoted and unquoted text", input: `foo"bar"baz`, want: []string{"foobarbaz"}},
		{name: "unterminated single quote", input: `don't worry`, wantErr: true},
		{name: "unterminated quote after escaped quote", input: `'test'\''ing`, wantErr: true},
		{name: "unterminated double quote", input: `"foo'bar`, wantErr: true},
		{name: "unterminated escape", input: `foo\`, wantErr: true},
		{name: "unterminated escape after whitespace", input: `   \`, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := splitCommandLine(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("splitCommandLine() error = %v, wantErr %v", err, test.wantErr)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("splitCommandLine() = %#v, want %#v", got, test.want)
			}
		})
	}
}
