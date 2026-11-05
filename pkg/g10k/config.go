package g10k

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

var (
	reModuledir = regexp.MustCompile(`^\s*(?:moduledir)\s+['\"]?([^'\"]+)['\"]?`)
)

// readConfigfile creates the ConfigSettings struct from the g10k config file
func (rt *Runtime) readConfigfile(configFile string) (ConfigSettings, error) {
	rt.Debugf("Trying to read g10k config file: " + configFile)
	data, err := os.ReadFile(configFile)
	if err != nil {
		return ConfigSettings{}, fmt.Errorf("readConfigfile(): There was an error parsing the config file %s: %s", configFile, err.Error())
	}

	rubySymbolsRemoved := ""
	for _, line := range strings.Split(string(data), "\n") {
		reWhitespaceColon := regexp.MustCompile(`^(\s*):`)
		m := reWhitespaceColon.FindStringSubmatch(line)
		if len(m) > 0 {
			rubySymbolsRemoved += reWhitespaceColon.ReplaceAllString(line, m[1]) + "\n"
		} else {
			rubySymbolsRemoved += line + "\n"
		}
	}
	var config ConfigSettings
	err = yaml.Unmarshal([]byte(rubySymbolsRemoved), &config)
	if err != nil {
		return ConfigSettings{}, fmt.Errorf("YAML unmarshal error: %s", err.Error())
	}

	if len(os.Getenv("g10k_cachedir")) > 0 {
		cachedir := os.Getenv("g10k_cachedir")
		rt.Debugf("Found environment variable g10k_cachedir set to: " + cachedir)
		config.CacheDir = rt.checkDirAndCreate(cachedir, "cachedir environment variable g10k_cachedir")
	} else {
		config.CacheDir = rt.checkDirAndCreate(config.CacheDir, "cachedir from g10k config "+configFile)
	}

	config.CacheDir = rt.checkDirAndCreate(config.CacheDir, "cachedir")
	config.ForgeCacheDir = rt.checkDirAndCreate(filepath.Join(config.CacheDir, "forge"), "cachedir/forge")
	config.ModulesCacheDir = rt.checkDirAndCreate(filepath.Join(config.CacheDir, "modules"), "cachedir/modules")
	config.EnvCacheDir = rt.checkDirAndCreate(filepath.Join(config.CacheDir, "environments"), "cachedir/environments")

	if len(config.ForgeBaseURL) == 0 {
		config.ForgeBaseURL = "https://forgeapi.puppet.com"
	}

	// fmt.Println("Forge Baseurl: ", config.ForgeBaseURL)

	// set default timeout to 5 seconds if no timeout setting found
	if config.Timeout == 0 {
		config.Timeout = 5
	}

	if rt.UseCacheFallback {
		config.UseCacheFallback = true
	}

	if rt.RetryGitCommands {
		config.RetryGitCommands = true
	}

	if rt.GitObjectSyntaxNotSupported {
		config.GitObjectSyntaxNotSupported = true
	}

	// set default max Go routines for Forge and Git module resolution if none is given
	if config.Maxworker <= 0 {
		config.Maxworker = rt.MaxWorker
	}
	if rt.MaxWorker != 50 {
		config.Maxworker = rt.MaxWorker
	}

	if rt.MaxWorker == 0 && config.Maxworker == 0 {
		config.Maxworker = 50
	}

	// set default max Go routines for Forge and Git module extracting
	if config.MaxExtractworker <= 0 {
		config.MaxExtractworker = rt.MaxExtractWorker
	}
	if rt.MaxExtractWorker != 20 {
		config.MaxExtractworker = rt.MaxExtractWorker
	}

	if rt.MaxExtractWorker == 0 && config.MaxExtractworker == 0 {
		config.MaxExtractworker = 20
	}

	if len(config.ForgeCacheTTLString) != 0 {
		ttl, err := time.ParseDuration(config.ForgeCacheTTLString)
		if err != nil {
			return ConfigSettings{}, fmt.Errorf("error: Can not convert value %s of config setting forge_cache_ttl to a golang Duration. valid time units are 300ms, 1.5h or 2h45m. In %s", config.ForgeCacheTTLString, configFile)
		}
		config.ForgeCacheTTL = ttl
	}

	// check for non-empty config.Deploy which takes precedence over the non-deploy scoped settings
	// See https://github.com/puppetlabs/r10k/blob/master/doc/dynamic-environments/configuration.mkd#deploy
	emptyDeploy := DeploySettings{}
	if !reflect.DeepEqual(config.Deploy, emptyDeploy) {
		rt.Debugf("detected deploy configuration hash, which takes precedence over the non-deploy scoped settings")
		config.PurgeLevels = config.Deploy.PurgeLevels
		config.PurgeAllowList = config.Deploy.PurgeAllowList
		config.DeploymentPurgeAllowList = config.Deploy.DeploymentPurgeAllowList
		config.WriteLock = config.Deploy.WriteLock
		config.GenerateTypes = config.Deploy.GenerateTypes
		config.PuppetPath = config.Deploy.PuppetPath
		config.PurgeSkiplist = config.Deploy.PurgeSkiplist
		config.Deploy = emptyDeploy
	}

	if len(config.PurgeLevels) == 0 {
		config.PurgeLevels = []string{"deployment", "puppetfile"}
	}

	for source, sa := range config.Sources {
		sa.Basedir = normalizeDir(sa.Basedir)

		// set default to "correct_and_warn" like r10k
		// https://github.com/puppetlabs/r10k/blob/master/doc/dynamic-environments/git-environments.mkd#invalid_branches
		if len(sa.AutoCorrectEnvironmentNames) == 0 {
			sa.AutoCorrectEnvironmentNames = "correct_and_warn"
		}
		config.Sources[source] = sa
	}

	if rt.Validate {
		if err := rt.Validatef(); err != nil {
			return ConfigSettings{}, err
		}
	}

	// fmt.Printf("%+v\n", config)
	return config, nil
}

// preparePuppetfile remove whitespace and comment lines from the given Puppetfile and merges Puppetfile resources that are identified with having a , at the end
func (rt *Runtime) preparePuppetfile(pf string) (string, error) {
	file, err := os.Open(pf)
	if err != nil {
		return "", fmt.Errorf("preparePuppetfile(): Error while opening Puppetfile %s Error: %s", pf, err.Error())
	}
	defer func() {
		_ = file.Close()
	}()

	reComma := regexp.MustCompile(`,\s*$`)
	reComment := regexp.MustCompile(`^\s*#`)
	reEmpty := regexp.MustCompile("^$")

	pfString := ""
	scanner := bufio.NewScanner(file)
	rt.Debugf("scanning file: " + pf)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !reComment.MatchString(line) && !reEmpty.MatchString(line) {
			if strings.Contains(line, "#") {
				rt.Debugf("found inline comment in " + pf + "line: " + line)
				line = strings.Split(line, "#")[0]
			}
			if reComma.MatchString(line) {
				pfString += line
				rt.Debugf("adding line:" + line)
			} else {
				pfString += line + "\n"
				rt.Debugf("adding line:" + line)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("preparePuppetfile(): Error while scanning Puppetfile %s Error: %s", pf, err.Error())
	}

	return pfString, nil
}

// readPuppetfile creates the Puppetfile struct using the goyacc parser.
func (rt *Runtime) readPuppetfile(pf string, sshKey string, source string, branch string, forceForgeVersions bool) (Puppetfile, error) {
	rt.Debugf("Trying to parse: " + pf)
	puppetFile, err := rt.ParsePuppetfile(pf, sshKey, source, branch, forceForgeVersions)
	if err != nil {
		return Puppetfile{}, err
	}
	if rt.Validate {
		if err := rt.Validatef(); err != nil {
			return Puppetfile{}, err
		}
	}
	return puppetFile, nil
}
