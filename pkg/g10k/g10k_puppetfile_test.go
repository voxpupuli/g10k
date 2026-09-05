package g10k

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func equalPuppetfile(rt *Runtime, a, b Puppetfile) bool {
	if &a == &b {
		return true
	}
	if a.forgeBaseURL != b.forgeBaseURL ||
		a.forgeCacheTTL != b.forgeCacheTTL ||
		a.privateKey != b.privateKey ||
		a.controlRepoBranch != b.controlRepoBranch ||
		a.source != b.source {
		rt.Debugf("forgeBaseURL, forgeCacheTTL, privateKey, controlRepoBranch or source isn't equal!")
		return false
	}

	if len(a.gitModules) != len(b.gitModules) ||
		len(a.forgeModules) != len(b.forgeModules) {
		rt.Debugf("size of gitModules or forgeModules isn't equal!")
		return false
	}

	for gitModuleName, gm := range a.gitModules {
		if _, ok := b.gitModules[gitModuleName]; !ok {
			rt.Debugf("git module " + gitModuleName + " missing!")
			return false
		}
		if !equalGitModule(gm, b.gitModules[gitModuleName]) {
			rt.Debugf("git module " + gitModuleName + " isn't equal!")
			return false
		}
	}

	for forgeModuleName, fm := range a.forgeModules {
		if _, ok := b.forgeModules[forgeModuleName]; !ok {
			rt.Debugf("forge module " + forgeModuleName + " missing!")
			return false
		}
		//fmt.Println("checking Forge module: ", forgeModuleName, fm)
		if !equalForgeModule(fm, b.forgeModules[forgeModuleName]) {
			rt.Debugf("forge module " + forgeModuleName + " isn't equal!")
			return false
		}
	}

	return true
}

func equalForgeResult(a, b ForgeResult) bool {
	if &a == &b {
		return true
	}
	if a.needToGet != b.needToGet || a.versionNumber != b.versionNumber ||
		a.fileSize != b.fileSize {
		return false
	}
	return true
}

func equalForgeModule(a, b ForgeModule) bool {
	if &a == &b {
		return true
	}
	if a.author != b.author || a.name != b.name ||
		a.version != b.version ||
		a.md5sum != b.md5sum ||
		a.sha256sum != b.sha256sum ||
		a.fileSize != b.fileSize ||
		a.baseURL != b.baseURL ||
		a.cacheTTL != b.cacheTTL {
		return false
	}
	return true
}

func equalGitModule(a, b GitModule) bool {
	if &a == &b {
		return true
	}
	if a.git != b.git ||
		a.privateKey != b.privateKey ||
		a.branch != b.branch ||
		a.tag != b.tag ||
		a.commit != b.commit ||
		a.ref != b.ref ||
		a.link != b.link ||
		a.ignoreUnreachable != b.ignoreUnreachable ||
		a.installPath != b.installPath ||
		a.local != b.local ||
		a.useSSHAgent != b.useSSHAgent {
		return false
	}
	if len(a.fallback) != len(b.fallback) {
		return false
	}
	for i, v := range a.fallback {
		if b.fallback[i] != v {
			return false
		}
	}
	return true
}

func readTestPuppetfile(t *testing.T, rt *Runtime, pf string, forceForgeVersions bool) Puppetfile {
	got, err := rt.readPuppetfile(pf, "", "test", "test", forceForgeVersions, false)
	if err != nil {
		t.Fatalf("readPuppetfile(%s): %v", pf, err)
	}
	return got
}

func checkExitCodeAndOutputOfReadPuppetfileSubprocess(t *testing.T, forceForgeVersions bool, expectedExitCode int, expectedOutput string) {
	pc, _, _, _ := runtime.Caller(1)
	testFunctionName := strings.Split(runtime.FuncForPC(pc).Name(), ".")[len(strings.Split(runtime.FuncForPC(pc).Name(), "."))-1]
	rt := NewRuntime(Options{})
	if os.Getenv("TEST_FOR_CRASH_"+testFunctionName) == "1" {
		if _, err := rt.readPuppetfile("../../tests/"+testFunctionName, "", "test", "test", forceForgeVersions, false); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run="+testFunctionName+"$")
	cmd.Env = append(os.Environ(), "TEST_FOR_CRASH_"+testFunctionName+"=1")
	out, err := cmd.CombinedOutput()
	if rt.Debug {
		fmt.Print(string(out))
	}

	exitCode := 0
	if msg, ok := err.(*exec.ExitError); ok { // there is error code
		exitCode = msg.Sys().(syscall.WaitStatus).ExitStatus()
	}

	if expectedExitCode != exitCode {
		t.Errorf("readPuppetfile() terminated with %v, but we expected exit status %v", exitCode, expectedExitCode)
	}
	if !strings.Contains(string(out), expectedOutput) {
		t.Errorf("readPuppetfile() terminated with the correct exit code, but the expected output was missing. out: %s", string(out))
	}
}

func TestPreparePuppetfile(t *testing.T) {
	expected := regexp.MustCompile("(moduledir 'external_modules'\nmod 'puppetlabs/ntp')")
	rt := NewRuntime(Options{})
	got, err := rt.preparePuppetfile("../../tests/TestPreparePuppetfile")
	if err != nil {
		t.Fatal(err)
	}

	if !expected.MatchString(got) {
		t.Error("Expected", expected, "got", got)
	}
}

func TestCommentPuppetfile(t *testing.T) {
	expected := regexp.MustCompile(`mod 'sensu',\s*:git => 'https://github.com/sensu/sensu-puppet.git',\s*:commit => '8f4fc5780071c4895dec559eafc6030511b0caaa'`)
	rt := NewRuntime(Options{})
	got, err := rt.preparePuppetfile("../../tests/TestCommentPuppetfile")
	if err != nil {
		t.Fatal(err)
	}

	assert.Regexp(t, expected, got)
}

func TestReadPuppetfile(t *testing.T) {
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	rt := NewRuntime(Options{})
	got := readTestPuppetfile(t, rt, "../../tests/"+funcName, false)

	fallbackMapExample := make([]string, 1)
	fallbackMapExample[0] = "master"

	fallbackMapExampleFull := make([]string, 3)
	fallbackMapExampleFull[0] = "b"
	fallbackMapExampleFull[1] = "a"
	fallbackMapExampleFull[2] = "r"

	fallbackMapAnother := make([]string, 4)
	fallbackMapAnother[0] = "dev"
	fallbackMapAnother[1] = "qa"
	fallbackMapAnother[2] = "prelive"
	fallbackMapAnother[3] = "live"

	gm := make(map[string]GitModule)
	gm["sensu"] = GitModule{git: "https://github.com/sensu/sensu-puppet.git",
		commit: "8f4fc5780071c4895dec559eafc6030511b0caaa", ignoreUnreachable: false}
	gm["example_module"] = GitModule{git: "git@somehost.com/foo/example-module.git",
		link: true, ignoreUnreachable: false, fallback: fallbackMapExample}
	gm["another_module"] = GitModule{git: "git@somehost.com/foo/another-module.git",
		link: true, ignoreUnreachable: false, fallback: fallbackMapAnother}
	gm["example_module_full"] = GitModule{git: "git@somehost.com/foo/example-module.git",
		branch: "foo", ignoreUnreachable: true, fallback: fallbackMapExampleFull}

	fm := make(map[string]ForgeModule)
	fm["apt"] = ForgeModule{version: "2.3.0", author: "puppetlabs", name: "apt"}
	fm["ntp"] = ForgeModule{version: "present", author: "puppetlabs", name: "ntp"}
	fm["stdlib"] = ForgeModule{version: "latest", author: "puppetlabs", name: "stdlib"}

	expected := Puppetfile{gitModules: gm, forgeModules: fm, source: "test", forgeCacheTTL: time.Duration(50 * time.Minute), forgeBaseURL: "foobar"}

	assert.True(t, equalPuppetfile(rt, got, expected))
}

func TestFallbackPuppetfile(t *testing.T) {
	fallbackMapExample := make([]string, 1)
	fallbackMapExample[0] = "master"

	fallbackMapAnother := make([]string, 4)
	fallbackMapAnother[0] = "dev"
	fallbackMapAnother[1] = "qa"
	fallbackMapAnother[2] = "prelive"
	fallbackMapAnother[3] = "live"

	gm := make(map[string]GitModule)
	gm["example_module"] = GitModule{git: "git@somehost.com/foo/example-module.git",
		link: true, ignoreUnreachable: false, fallback: fallbackMapExample}
	gm["another_module"] = GitModule{git: "git@somehost.com/foo/another-module.git",
		branch: "master", ignoreUnreachable: false, fallback: fallbackMapAnother}

	expected := Puppetfile{gitModules: gm, source: "test"}
	rt := NewRuntime(Options{})
	got := readTestPuppetfile(t, rt, "../../tests/TestFallbackPuppetfile", false)

	if !equalGitModule(got.gitModules["example_module"], expected.gitModules["example_module"]) {
		t.Error("Expected gitModules:", expected.gitModules["example_module"], ", but got gitModules:", got.gitModules["example_module"])
	}

	if !equalGitModule(got.gitModules["another_module"], expected.gitModules["another_module"]) {
		t.Error("Expected gitModules:", expected.gitModules["another_module"], ", but got gitModules:", got.gitModules["another_module"])
	}
}

func TestForgeCacheTTLPuppetfile(t *testing.T) {
	expected := regexp.MustCompile("(moduledir 'external_modules'\nforge.cacheTtl 50m\n)")
	rt := NewRuntime(Options{})
	got, err := rt.preparePuppetfile("../../tests/TestForgeCacheTTLPuppetfile")
	if err != nil {
		t.Fatal(err)
	}

	if !expected.MatchString(got) {
		t.Error("Expected", expected, "got", got)
	}

	expectedPuppetfile := Puppetfile{forgeCacheTTL: 50 * time.Minute}
	gotPuppetfile := readTestPuppetfile(t, rt, "../../tests/TestForgeCacheTTLPuppetfile", false)

	if gotPuppetfile.forgeCacheTTL != expectedPuppetfile.forgeCacheTTL {
		t.Error("Expected for forgeCacheTTL", expectedPuppetfile.forgeCacheTTL, "got", gotPuppetfile.forgeCacheTTL)
	}

}

func TestForceForgeVersionsPuppetfile(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, true, 1, "")
}

func TestForceForgeVersionsPuppetfileCorrect(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, true, 0, "")
}

func TestReadPuppetfileDuplicateGitAttribute(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileTrailingComma(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileInvalidForgeModuleName(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileDuplicateForgeModule(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileMissingGitAttribute(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileTooManyGitAttributes(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileConflictingGitAttributesTag(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileConflictingGitAttributesBranch(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileConflictingGitAttributesCommit(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileConflictingGitAttributesRef(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileIgnoreUnreachable(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileForgeCacheTTL(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "error: Can not convert value 300x of parameter forge.cacheTtl 300x to a golang Duration. valid time units are 300ms, 1.5h or 2h45m. In ../../tests/TestReadPuppetfileForgeCacheTTL line: forge.cacheTtl 300x")
}

func TestReadPuppetfileLink(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "error: found conflicting git attributes :branch, :link, in ../../tests/TestReadPuppetfileLink for module example_module line: mod 'example_module',:git => 'git@somehost.com/foo/example-module.git',:branch => 'foo',:link => true")
}

func TestReadPuppetfileDuplicateForgeGitModule(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "error: git puppet module with same name found in ../../tests/TestReadPuppetfileDuplicateForgeGitModule for module bar line: mod 'bar',:git => 'https://github.com/foo/bar.git'")
}

func TestReadPuppetfileChecksumAttribute(t *testing.T) {
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	rt := NewRuntime(Options{})
	got := readTestPuppetfile(t, rt, "../../tests/"+funcName, false)

	fm := make(map[string]ForgeModule)
	fm["ntp"] = ForgeModule{version: "6.0.0", author: "puppetlabs", name: "ntp", sha256sum: "a988a172a3edde6ac2a26d0e893faa88d37bc47465afc50d55225a036906c944"}
	fm["stdlib"] = ForgeModule{version: "2.3.0", author: "puppetlabs", name: "stdlib", sha256sum: "433c69fb99a46185e81619fadb70e0961bce2f4e952294a16e61364210d1519d"}
	fm["apt"] = ForgeModule{version: "2.3.0", author: "puppetlabs", name: "apt", sha256sum: "a09290c207bbfed7f42dd0356ff4dee16e138c7f9758d2134a21aeb66e14072f"}
	fm["concat"] = ForgeModule{version: "2.2.0", author: "puppetlabs", name: "concat", sha256sum: "ec0407abab71f57e106ade6ed394410d08eec29bdad4c285580e7b56514c5194"}

	expected := Puppetfile{forgeModules: fm, source: "test"}

	assert.True(t, equalPuppetfile(rt, got, expected))
}

func TestReadPuppetfileForgeSlashNotation(t *testing.T) {
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]

	rt := NewRuntime(Options{})
	got := readTestPuppetfile(t, rt, "../../tests/"+funcName, false)
	fm := make(map[string]ForgeModule)
	fm["filebeat"] = ForgeModule{version: "0.10.4", author: "pcfens", name: "filebeat"}
	expected := Puppetfile{forgeModules: fm, source: "test"}
	assert.True(t, equalPuppetfile(rt, got, expected))

}

func TestReadPuppetfileForgeDash(t *testing.T) {
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	rt := NewRuntime(Options{})
	got := readTestPuppetfile(t, rt, "../../tests/"+funcName, false)

	fm := make(map[string]ForgeModule)
	fm["php"] = ForgeModule{version: "4.0.0-beta1", author: "mayflower", name: "php"}

	expected := Puppetfile{forgeModules: fm, source: "test"}

	assert.True(t, equalPuppetfile(rt, got, expected))
}

func TestReadPuppetfileInstallPath(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readTestPuppetfile(t, rt, "../../tests/"+funcName, false)

	gm := make(map[string]GitModule)
	gm["sensu"] = GitModule{git: "https://github.com/sensu/sensu-puppet.git", commit: "8f4fc5780071c4895dec559eafc6030511b0caaa", installPath: "external"}

	expected := Puppetfile{gitModules: gm, source: "test"}
	//fmt.Println(got)

	assert.True(t, equalPuppetfile(rt, got, expected))
}

func TestReadPuppetfileLocalModule(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readTestPuppetfile(t, rt, "../../tests/"+funcName, false)

	gm := make(map[string]GitModule)
	gm["localstuff"] = GitModule{local: true}
	gm["localstuff2"] = GitModule{local: true}
	gm["localstuff3"] = GitModule{local: false}
	gm["external"] = GitModule{local: true, installPath: "modules"}

	expected := Puppetfile{source: "test", gitModules: gm}
	//fmt.Println(got)

	assert.True(t, equalPuppetfile(rt, got, expected))
}

func TestReadPuppetfileMissingTrailingComma(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileMissingTrailingComma2(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileForgeNotationGitModule(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readTestPuppetfile(t, rt, "../../tests/"+funcName, false)

	gm := make(map[string]GitModule)
	gm["elasticsearch"] = GitModule{git: "https://github.com/elastic/puppet-elasticsearch.git", branch: "5.x"}

	expected := Puppetfile{source: "test", gitModules: gm}
	//fmt.Println(got)

	assert.True(t, equalPuppetfile(rt, got, expected))
}

func TestReadPuppetfileGitSlashNotation(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readTestPuppetfile(t, rt, "../../tests/"+funcName, false)

	fm := make(map[string]ForgeModule)
	fm["stdlib"] = ForgeModule{version: "present", author: "puppetlabs", name: "stdlib"}
	fm["apache"] = ForgeModule{version: "present", author: "puppetlabs", name: "apache"}
	fm["apt"] = ForgeModule{version: "latest", author: "puppetlabs", name: "apt"}
	fm["rsync"] = ForgeModule{version: "latest", author: "puppetlabs", name: "rsync"}

	gm := make(map[string]GitModule)
	gm["puppetboard"] = GitModule{git: "https://github.com/nibalizer/puppet-module-puppetboard.git", ref: "2.7.1"}
	gm["elasticsearch"] = GitModule{git: "https://github.com/alexharv074/puppet-elasticsearch.git", ref: "alex_master"}

	expected := Puppetfile{source: "test", gitModules: gm, forgeModules: fm}
	//fmt.Println(got)

	assert.True(t, equalPuppetfile(rt, got, expected))
}

func TestReadPuppetfileGitDashNotation(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readTestPuppetfile(t, rt, "../../tests/"+funcName, false)

	fm := make(map[string]ForgeModule)
	fm["stdlib"] = ForgeModule{version: "present", author: "puppetlabs", name: "stdlib"}
	fm["apache"] = ForgeModule{version: "present", author: "puppetlabs", name: "apache"}
	fm["apt"] = ForgeModule{version: "latest", author: "puppetlabs", name: "apt"}
	fm["rsync"] = ForgeModule{version: "latest", author: "puppetlabs", name: "rsync"}

	gm := make(map[string]GitModule)
	gm["puppetboard"] = GitModule{git: "https://github.com/nibalizer/puppet-module-puppetboard.git", ref: "2.7.1"}
	gm["elasticsearch"] = GitModule{git: "https://github.com/alexharv074/puppet-elasticsearch.git", ref: "alex_master"}

	expected := Puppetfile{source: "test", gitModules: gm, forgeModules: fm}
	//fmt.Println(got)

	assert.True(t, equalPuppetfile(rt, got, expected))
}

func TestReadPuppetfileGitDashNSlashNotation(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readTestPuppetfile(t, rt, "../../tests/"+funcName, false)

	fm := make(map[string]ForgeModule)
	fm["stdlib"] = ForgeModule{version: "present", author: "puppetlabs", name: "stdlib"}
	fm["apache"] = ForgeModule{version: "present", author: "puppetlabs", name: "apache"}
	fm["apt"] = ForgeModule{version: "latest", author: "puppetlabs", name: "apt"}
	fm["rsync"] = ForgeModule{version: "latest", author: "puppetlabs", name: "rsync"}

	gm := make(map[string]GitModule)
	gm["puppetboard"] = GitModule{git: "https://github.com/nibalizer/puppet-module-puppetboard.git", ref: "2.7.1"}
	gm["elasticsearch"] = GitModule{git: "https://github.com/alexharv074/puppet-elasticsearch.git", ref: "alex_master"}

	expected := Puppetfile{source: "test", gitModules: gm, forgeModules: fm}
	//fmt.Println(got)

	assert.True(t, equalPuppetfile(rt, got, expected))
}

func TestReadPuppetfileSSHKeyAlreadyLoaded(t *testing.T) {
	rt := NewRuntime(Options{Quiet: true})
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readTestPuppetfile(t, rt, "../../tests/"+funcName, false)

	fm := make(map[string]ForgeModule)
	gm := make(map[string]GitModule)
	gm["example_module"] = GitModule{git: "git@somehost.com/foo/example-module.git", branch: "foo", useSSHAgent: true}

	expected := Puppetfile{source: "test", gitModules: gm, forgeModules: fm}
	//fmt.Println(got)

	assert.True(t, equalPuppetfile(rt, got, expected))
}
