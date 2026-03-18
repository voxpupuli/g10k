package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
)

func equalPuppetfile(a, b Puppetfile) bool {
	if &a == &b {
		return true
	}
	if a.forgeBaseURL != b.forgeBaseURL ||
		a.forgeCacheTTL != b.forgeCacheTTL ||
		a.privateKey != b.privateKey ||
		a.controlRepoBranch != b.controlRepoBranch ||
		a.source != b.source {
		Debugf("forgeBaseURL, forgeCacheTTL, privateKey, controlRepoBranch or source isn't equal!")
		return false
	}

	if len(a.gitModules) != len(b.gitModules) ||
		len(a.forgeModules) != len(b.forgeModules) {
		Debugf("size of gitModules or forgeModules isn't equal!")
		return false
	}

	for gitModuleName, gm := range a.gitModules {
		if _, ok := b.gitModules[gitModuleName]; !ok {
			Debugf("git module " + gitModuleName + " missing!")
			return false
		}
		if !equalGitModule(gm, b.gitModules[gitModuleName]) {
			Debugf("git module " + gitModuleName + " isn't equal!")
			return false
		}
	}

	for forgeModuleName, fm := range a.forgeModules {
		if _, ok := b.forgeModules[forgeModuleName]; !ok {
			Debugf("forge module " + forgeModuleName + " missing!")
			return false
		}
		//fmt.Println("checking Forge module: ", forgeModuleName, fm)
		if !equalForgeModule(fm, b.forgeModules[forgeModuleName]) {
			Debugf("forge module " + forgeModuleName + " isn't equal!")
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

func checkExitCodeAndOutputOfReadPuppetfileSubprocess(t *testing.T, forceForgeVersions bool, expectedExitCode int, expectedOutput string) {
	pc, _, _, _ := runtime.Caller(1)
	testFunctionName := strings.Split(runtime.FuncForPC(pc).Name(), ".")[len(strings.Split(runtime.FuncForPC(pc).Name(), "."))-1]
	if os.Getenv("TEST_FOR_CRASH_"+testFunctionName) == "1" {
		readPuppetfile("tests/"+testFunctionName, "", "test", "test", forceForgeVersions, false)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run="+testFunctionName+"$")
	cmd.Env = append(os.Environ(), "TEST_FOR_CRASH_"+testFunctionName+"=1")
	out, err := cmd.CombinedOutput()
	if debug {
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
	got := preparePuppetfile("tests/TestPreparePuppetfile")

	if !expected.MatchString(got) {
		t.Error("Expected", expected, "got", got)
	}
}

func TestCommentPuppetfile(t *testing.T) {
	expected := regexp.MustCompile(`mod 'sensu',\s*:git => 'https://github.com/sensu/sensu-puppet.git',\s*:commit => '8f4fc5780071c4895dec559eafc6030511b0caaa'`)
	got := preparePuppetfile("tests/TestCommentPuppetfile")

	if !expected.MatchString(got) {
		spew.Dump(expected)
		spew.Dump(got)
		t.Error("Expected", expected, "got", got)
	}
}

func TestReadPuppetfile(t *testing.T) {
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readPuppetfile("tests/"+funcName, "", "test", "test", false, false)

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

	if !equalPuppetfile(got, expected) {
		spew.Dump(expected)
		spew.Dump(got)
		t.Error("Expected Puppetfile:", expected, ", but got Puppetfile:", got)
	}
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
	got := readPuppetfile("tests/TestFallbackPuppetfile", "", "test", "test", false, false)

	if !equalGitModule(got.gitModules["example_module"], expected.gitModules["example_module"]) {
		t.Error("Expected gitModules:", expected.gitModules["example_module"], ", but got gitModules:", got.gitModules["example_module"])
	}

	if !equalGitModule(got.gitModules["another_module"], expected.gitModules["another_module"]) {
		t.Error("Expected gitModules:", expected.gitModules["another_module"], ", but got gitModules:", got.gitModules["another_module"])
	}
}

func TestForgeCacheTTLPuppetfile(t *testing.T) {
	expected := regexp.MustCompile("(moduledir 'external_modules'\nforge.cacheTtl 50m\n)")
	got := preparePuppetfile("tests/TestForgeCacheTTLPuppetfile")

	if !expected.MatchString(got) {
		t.Error("Expected", expected, "got", got)
	}

	expectedPuppetfile := Puppetfile{forgeCacheTTL: 50 * time.Minute}
	gotPuppetfile := readPuppetfile("tests/TestForgeCacheTTLPuppetfile", "", "test", "test", false, false)

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
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "Error: Can not convert value 300x of parameter forge.cacheTtl 300x to a golang Duration. Valid time units are 300ms, 1.5h or 2h45m. In tests/TestReadPuppetfileForgeCacheTTL line: forge.cacheTtl 300x")
}

func TestReadPuppetfileLink(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "Error: Found conflicting git attributes :branch, :link, in tests/TestReadPuppetfileLink for module example_module line: mod 'example_module',:git => 'git@somehost.com/foo/example-module.git',:branch => 'foo',:link => true")
}

func TestReadPuppetfileDuplicateForgeGitModule(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "Error: Git Puppet module with same name found in tests/TestReadPuppetfileDuplicateForgeGitModule for module bar line: mod 'bar',:git => 'https://github.com/foo/bar.git'")
}

func TestReadPuppetfileChecksumAttribute(t *testing.T) {
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readPuppetfile("tests/"+funcName, "", "test", "test", false, false)

	fm := make(map[string]ForgeModule)
	fm["ntp"] = ForgeModule{version: "6.0.0", author: "puppetlabs", name: "ntp", sha256sum: "a988a172a3edde6ac2a26d0e893faa88d37bc47465afc50d55225a036906c944"}
	fm["stdlib"] = ForgeModule{version: "2.3.0", author: "puppetlabs", name: "stdlib", sha256sum: "433c69fb99a46185e81619fadb70e0961bce2f4e952294a16e61364210d1519d"}
	fm["apt"] = ForgeModule{version: "2.3.0", author: "puppetlabs", name: "apt", sha256sum: "a09290c207bbfed7f42dd0356ff4dee16e138c7f9758d2134a21aeb66e14072f"}
	fm["concat"] = ForgeModule{version: "2.2.0", author: "puppetlabs", name: "concat", sha256sum: "ec0407abab71f57e106ade6ed394410d08eec29bdad4c285580e7b56514c5194"}

	expected := Puppetfile{forgeModules: fm, source: "test"}

	if !equalPuppetfile(got, expected) {
		spew.Dump(expected)
		spew.Dump(got)
		t.Error("Expected Puppetfile:", expected, ", but got Puppetfile:", got)
	}
}

func TestReadPuppetfileForgeSlashNotation(t *testing.T) {
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]

	got := readPuppetfile("tests/"+funcName, "", "test", "test", false, false)
	fm := make(map[string]ForgeModule)
	fm["filebeat"] = ForgeModule{version: "0.10.4", author: "pcfens", name: "filebeat"}
	expected := Puppetfile{forgeModules: fm, source: "test"}
	if !equalPuppetfile(got, expected) {
		spew.Dump(expected)
		spew.Dump(got)
		t.Error("Expected Puppetfile:", expected, ", but got Puppetfile:", got)
	}

}

func TestReadPuppetfileForgeDash(t *testing.T) {
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readPuppetfile("tests/"+funcName, "", "test", "test", false, false)

	fm := make(map[string]ForgeModule)
	fm["php"] = ForgeModule{version: "4.0.0-beta1", author: "mayflower", name: "php"}

	expected := Puppetfile{forgeModules: fm, source: "test"}

	if !equalPuppetfile(got, expected) {
		spew.Dump(expected)
		spew.Dump(got)
		t.Errorf("Expected Puppetfile: %+v, but got Puppetfile: %+v", expected, got)
	}
}

func TestReadPuppetfileInstallPath(t *testing.T) {
	quiet = true
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readPuppetfile("tests/"+funcName, "", "test", "test", false, false)

	gm := make(map[string]GitModule)
	gm["sensu"] = GitModule{git: "https://github.com/sensu/sensu-puppet.git", commit: "8f4fc5780071c4895dec559eafc6030511b0caaa", installPath: "external"}

	expected := Puppetfile{gitModules: gm, source: "test"}
	//fmt.Println(got)

	if !equalPuppetfile(got, expected) {
		spew.Dump(expected)
		spew.Dump(got)
		t.Errorf("Expected Puppetfile: %+v, but got Puppetfile: %+v", expected, got)
	}
}

func TestReadPuppetfileLocalModule(t *testing.T) {
	quiet = true
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readPuppetfile("tests/"+funcName, "", "test", "test", false, false)

	gm := make(map[string]GitModule)
	gm["localstuff"] = GitModule{local: true}
	gm["localstuff2"] = GitModule{local: true}
	gm["localstuff3"] = GitModule{local: false}
	gm["external"] = GitModule{local: true, installPath: "modules"}

	expected := Puppetfile{source: "test", gitModules: gm}
	//fmt.Println(got)

	if !equalPuppetfile(got, expected) {
		spew.Dump(expected)
		spew.Dump(got)
		t.Errorf("Expected Puppetfile: %+v, but got Puppetfile: %+v", expected, got)
	}
}

func TestReadPuppetfileMissingTrailingComma(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileMissingTrailingComma2(t *testing.T) {
	checkExitCodeAndOutputOfReadPuppetfileSubprocess(t, false, 1, "")
}

func TestReadPuppetfileForgeNotationGitModule(t *testing.T) {
	quiet = true
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readPuppetfile("tests/"+funcName, "", "test", "test", false, false)

	gm := make(map[string]GitModule)
	gm["elasticsearch"] = GitModule{git: "https://github.com/elastic/puppet-elasticsearch.git", branch: "5.x"}

	expected := Puppetfile{source: "test", gitModules: gm}
	//fmt.Println(got)

	if !equalPuppetfile(got, expected) {
		spew.Dump(expected)
		spew.Dump(got)
		t.Errorf("Expected Puppetfile: %+v, but got Puppetfile: %+v", expected, got)
	}
}

func TestReadPuppetfileGitSlashNotation(t *testing.T) {
	quiet = true
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readPuppetfile("tests/"+funcName, "", "test", "test", false, false)

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

	if !equalPuppetfile(got, expected) {
		spew.Dump(expected)
		spew.Dump(got)
		t.Errorf("Expected Puppetfile: %+v, but got Puppetfile: %+v", expected, got)
	}
}

func TestReadPuppetfileGitDashNotation(t *testing.T) {
	quiet = true
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readPuppetfile("tests/"+funcName, "", "test", "test", false, false)

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

	if !equalPuppetfile(got, expected) {
		spew.Dump(expected)
		spew.Dump(got)
		t.Errorf("Expected Puppetfile: %+v, but got Puppetfile: %+v", expected, got)
	}
}

func TestReadPuppetfileGitDashNSlashNotation(t *testing.T) {
	quiet = true
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readPuppetfile("tests/"+funcName, "", "test", "test", false, false)

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

	if !equalPuppetfile(got, expected) {
		spew.Dump(expected)
		spew.Dump(got)
		t.Errorf("Expected Puppetfile: %+v, but got Puppetfile: %+v", expected, got)
	}
}

func TestReadPuppetfileSSHKeyAlreadyLoaded(t *testing.T) {
	quiet = true
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]
	got := readPuppetfile("tests/"+funcName, "", "test", "test", false, false)

	fm := make(map[string]ForgeModule)
	gm := make(map[string]GitModule)
	gm["example_module"] = GitModule{git: "git@somehost.com/foo/example-module.git", branch: "foo", useSSHAgent: true}

	expected := Puppetfile{source: "test", gitModules: gm, forgeModules: fm}
	//fmt.Println(got)

	if !equalPuppetfile(got, expected) {
		fmt.Println("Expected:")
		spew.Dump(expected)
		fmt.Println("Got:")
		spew.Dump(got)
		t.Errorf("Expected Puppetfile: %+v, but got Puppetfile: %+v", expected, got)
	}
}

func TestResolvePuppetfileMatch(t *testing.T) {
	quiet = true
	funcName := strings.Split(funcName(), ".")[len(strings.Split(funcName(), "."))-1]

	// Create a dummy git repo
	repoDir := "tests/test-control-repo"
	repoURL := "file://" + filepath.Join(os.Getenv("PWD"), repoDir)

	// Ensure clean state
	purgeDir(repoDir, funcName)
	err := os.MkdirAll(repoDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init", repoDir)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Use fake forge to prevent network calls and timeout
	ts := spinUpFakeForge(t, "tests/fake-forge/latest-puppetlabs-ntp-metadata.json")
	defer ts.Close()

	// Create Puppetfile
	pfContent := []byte(`mod 'puppetlabs/ntp', '6.0.0'`)
	if err := os.WriteFile(filepath.Join(repoDir, "Puppetfile"), pfContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Commit
	cmd = exec.Command("git", "-C", repoDir, "add", "Puppetfile")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "-C", repoDir, "config", "user.email", "you@example.com")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "-C", repoDir, "config", "user.name", "Your Name")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	cmd = exec.Command("git", "-C", repoDir, "commit", "-m", "Initial commit")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Setup config
	config = ConfigSettings{
		CacheDir:        "/tmp/g10k-test-cache",
		ForgeCacheDir:   "/tmp/g10k-test-cache/forge",
		ModulesCacheDir: "/tmp/g10k-test-cache/modules",
		EnvCacheDir:     "/tmp/g10k-test-cache/environments",
		ForgeBaseURL:    ts.URL,
		Sources: map[string]Source{
			"test": {
				Remote:  repoURL,
				Basedir: "/tmp/g10k-test-envs",
				Prefix:  "false",
			},
		},
		MaxExtractworker: 1,
		Maxworker:        5,
	}

	// Clean up previous runs
	purgeDir(config.CacheDir, funcName)
	purgeDir(config.Sources["test"].Basedir, funcName)
	// Create cache directories that g10k expects to exist
	if err := os.MkdirAll(config.ForgeCacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.ModulesCacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.EnvCacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// First run: should deploy
	branchParam = "master"
	resolvePuppetEnvironment(false, "")

	// Verify deployment
	deployedPF := filepath.Join(config.Sources["test"].Basedir, "master", "Puppetfile")
	if !fileExists(deployedPF) {
		t.Errorf("Puppetfile not deployed")
	}

	// Verify checksum in deployed file
	deployFile := filepath.Join(config.Sources["test"].Basedir, "master", ".g10k-deploy.json")
	if !fileExists(deployFile) {
		t.Errorf(".g10k-deploy.json not created")
	}

	// Second run: should detect match and skip module resolution
	// We can't easily assert on internal log messages without capturing stdout/log,
	// but we can check if the skipped logic is hit by ensuring no errors occur and fast execution.
	// Ideally we would mock the module resolution to fail if called, but that's hard here.

	// Let's modify the deployed Puppetfile to see if it gets overwritten (it shouldn't if skipped)
	// Wait, if it skips resolution, it won't even read the Puppetfile to know what modules to fetch.
	// If we were to modify the deployed Puppetfile, g10k might notice the checksum mismatch?
	// The optimization is: upstream git content == deployed file content on disk.

	// Ensure we are in a clean state for variables
	needSyncEnvs = make(map[string]struct{})

	// Capture log output
	// var buf bytes.Buffer
	// log.SetOutput(&buf)
	// defer func() {
	// 	log.SetOutput(os.Stderr)
	// }()

	// Enable debug logging
	// debug = true
	// defer func() { debug = false }()

	// Second run: should detect match and skip module resolution
	// Ensure we are in a clean state for variables
	needSyncEnvs = make(map[string]struct{})

	// Capture log output to verify the optimization
	var buf strings.Builder
	log.SetOutput(&buf)
	debug = true
	verbose = true

	// Run the resolution again
	resolvePuppetEnvironment(false, "")

	// Restore logging
	debug = false
	verbose = false
	log.SetOutput(os.Stdout)

	// Verify that the optimization was triggered
	logOutput := buf.String()
	expectedMsg := "Skipping resolution of branch master of source test because Puppetfile content has not changed"
	if !strings.Contains(logOutput, expectedMsg) {
		t.Errorf("Expected optimization message not found in logs.\nExpected: %s\nGot:\n%s", expectedMsg, logOutput)
	}

	// Cleanup
	purgeDir(repoDir, funcName)
}
