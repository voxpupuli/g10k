package puppetfileparser_test

import "testing"

func TestModuleDirDeclarations(t *testing.T) {
	checkEvents(t, "moduledir 'modules'\r\nmoduledir \"other/\"\r\n", []event{
		{kind: "moduledir", value: "modules"},
		{kind: "moduledir", value: "other/"},
	})
}
