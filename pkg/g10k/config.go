package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
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

// readPuppetfile creates the ConfigSettings struct from the Puppetfile
func (rt *Runtime) readPuppetfile(pf string, sshKey string, source string, branch string, forceForgeVersions bool, replacedPuppetfileContent bool) (Puppetfile, error) {
	var puppetFile Puppetfile
	var n string
	puppetFile.privateKey = sshKey
	puppetFile.source = source
	puppetFile.forgeModules = map[string]ForgeModule{}
	puppetFile.gitModules = map[string]GitModule{}
	if replacedPuppetfileContent {
		rt.Debugf("Using replaced Puppetfile content, probably because a Git module was found in Forge notation")
		n = pf
	} else {
		rt.Debugf("Trying to parse: " + pf)
		var err error
		n, err = rt.preparePuppetfile(pf)
		if err != nil {
			return Puppetfile{}, fmt.Errorf("error preparing Puppetfile: %s", err.Error())
		}
	}

	reEmptyLine := regexp.MustCompile(`^\s*$`)
	reForgeCacheTTL := regexp.MustCompile(`^\s*(?:forge.cache(?:TTL|Ttl))\s+['\"]?([^'\"]+)['\"]?`)
	reForgeBaseURL := regexp.MustCompile(`^\s*(?:forge.base(?:URL|Url))\s+['\"]?([^'\"]+)['\"]?`)
	reForgeModule := regexp.MustCompile(`^\s*(?:mod)\s+['\"]?([^'\"]+[-/][^'\"]+)['\"](?:\s*)[,]?(.*)`)
	reForgeAttribute := regexp.MustCompile(`\s*['\"]?([^\s'\"]+)\s*['\"]?(?:=>)?\s*['\"]?([^'\"]+)?`)
	reGitModule := regexp.MustCompile(`^\s*(?:mod)\s+['\"]?([^'\"/]+)['\"]\s*,(.*)`)
	reGitAttribute := regexp.MustCompile(`\s*:(git|commit|tag|branch|ref|link|ignore[-_]unreachable|fallback|install_path|default_branch|local|use_ssh_agent)\s*=>\s*['\"]?([^'\"]+)['\"]?`)
	reUniqueGitAttribute := regexp.MustCompile(`\s*:(?:commit|tag|branch|ref|link)\s*=>`)
	reDanglingAttribute := regexp.MustCompile(`^\s*:[^ ]+\s*=>`)
	moduleDir := "modules"
	// moduledir CLI parameter override
	if len(rt.ModuleDir) != 0 {
		moduleDir = rt.ModuleDir
	}
	var moduleDirs []string
	//nextLineAttr := false

	lines := strings.Split(n, "\n")
	for i, line := range lines {
		//fmt.Println("found line ---> ", line, "$")
		if m := reEmptyLine.FindStringSubmatch(line); len(m) > 0 {
			continue
		}
		if strings.Count(line, ":git") > 1 || strings.Count(line, ":tag") > 1 || strings.Count(line, ":branch") > 1 || strings.Count(line, ":ref") > 1 || strings.Count(line, ":link") > 1 {
			return Puppetfile{}, fmt.Errorf("error: trailing comma found in %s somewhere here: %s", pf, line)
		}
		if m := reDanglingAttribute.FindStringSubmatch(line); len(m) >= 1 {
			previousLine := ""
			if i-1 >= 0 {
				previousLine = lines[i-1]
			}
			return Puppetfile{}, fmt.Errorf("error: found dangling module attribute in %s somewhere here: %s%s check for missing , at the end of the line", pf, previousLine, line)
		}
		if m := reModuledir.FindStringSubmatch(line); len(m) > 1 && len(rt.ModuleDir) == 0 {
			moduleDir = normalizeDir(m[1])
			moduleDirs = append(moduleDirs, moduleDir)
		} else if m := reForgeBaseURL.FindStringSubmatch(line); len(m) > 1 {
			puppetFile.forgeBaseURL = m[1]
			//fmt.Println("found forge base URL parameter ---> ", m[1])
		} else if m := reForgeCacheTTL.FindStringSubmatch(line); len(m) > 1 {
			ttl, err := time.ParseDuration(m[1])
			if err != nil {
				return Puppetfile{}, fmt.Errorf("error: Can not convert value %s of parameter %s to a golang Duration. valid time units are 300ms, 1.5h or 2h45m. In %s line: %s", m[1], m[0], pf, line)
			}
			puppetFile.forgeCacheTTL = ttl
		} else if m := reForgeModule.FindStringSubmatch(line); len(m) > 1 {
			forgeModuleName := strings.TrimSpace(m[1])
			//fmt.Println("found forge mod name ------------------------------> ", forgeModuleName)
			comp := strings.Split(forgeModuleName, "/")
			forgeModuleNameSeparator := "/"
			if len(comp) != 2 {
				comp = strings.Split(forgeModuleName, "-")
				forgeModuleNameSeparator = "-"
				if len(comp) != 2 {
					return Puppetfile{}, fmt.Errorf("error: Forge module name is invalid! Should be like puppetlabs/apt or puppetlabs-apt, but is: %s in %s line: %s", m[2], pf, line)
				}
			}
			forgeModuleName = comp[0] + "/" + comp[1]
			if _, ok := puppetFile.forgeModules[comp[1]]; ok {
				return Puppetfile{}, fmt.Errorf("error: Duplicate forge module found in %s for module %s line: %s", pf, forgeModuleName, line)
			}
			//Debugf("Found Forge module name " + forgeModuleName + " with " + forgeModuleNameSeparator + " as a separator")
			forgeModuleVersion := "present"
			forgeChecksum := ""
			// try to find a forge module attribute
			if len(m[2]) > 1 {
				forgeModuleAttributes := m[2]
				forgeModuleAttributesArray := strings.Split(forgeModuleAttributes, ",")
				//fmt.Println("found forge mod attribute array ---> ", forgeModuleAttributesArray)
				//fmt.Println("len(forgeModuleAttributesArray) --> ", len(forgeModuleAttributesArray))
				for i := 0; i <= strings.Count(forgeModuleAttributes, ","); i++ {
					a := reForgeAttribute.FindStringSubmatch(forgeModuleAttributesArray[i])
					//fmt.Println("a[1] ---> ", a[1])
					forgeAttribute := strings.Replace(strings.TrimSpace(a[1]), ":", "", 1)
					if forgeAttribute != "sha256sum" {
						forgeModuleVersion = forgeAttribute
						rt.Debugf("setting forge module " + forgeModuleName + " to version " + forgeModuleVersion)
					}
					if len(a[2]) > 1 {
						//fmt.Println("a[2] ---> ", a[2])
						forgeAttributeName := strings.TrimSpace(a[1])
						forgeAttributeValue := strings.TrimSpace(a[2])
						rt.Debugf("found forge attribute ---> " + forgeAttributeName + " with value ---> " + forgeAttributeValue)
						if forgeAttributeName == ":sha256sum" {
							forgeChecksum = forgeAttributeValue
						} else if forgeAttribute == "git" {
							// try to detect Git modules in Forge <AUTHOR>/<MODULENAME> notation, fixes #104
							rt.Debugf("Found git module in Forge notation: " + forgeModuleName + " with git url: " + forgeAttributeValue)
							//fmt.Println("line:", line)
							removeForgeNotationAuthor := strings.Split(line, forgeModuleNameSeparator)
							if len(removeForgeNotationAuthor) < 2 {
								return Puppetfile{}, fmt.Errorf("error: Found git module in Forge notation: %s with git url: %s, but something went wrong while trying to remove the author part to make g10k detect it as an Git module module:%s line: %s", forgeModuleName, forgeAttributeValue, comp[1], line)
							} else {
								//fmt.Println("removeForgeNotationAuthor:", removeForgeNotationAuthor[0])
								replacedLine := strings.Replace(line, removeForgeNotationAuthor[0]+forgeModuleNameSeparator, "mod '", 1)
								//fmt.Println("replacedLine:", replacedLine)
								//fmt.Print("n:", n)
								newN := strings.Replace(n, line, replacedLine, 1)
								//fmt.Print("newN:", newN)
								return rt.readPuppetfile(newN, sshKey, source, branch, forceForgeVersions, true)
							}
						}
					}
				}
			}
			if forceForgeVersions && (forgeModuleVersion == "present" || forgeModuleVersion == "latest") {
				return Puppetfile{}, fmt.Errorf("error: Found %s setting for forge module in %s for module %s line: %s and force_forge_versions is set to true! Please specify a version (e.g. '2.3.0')", forgeModuleVersion, pf, forgeModuleName, line)
			}
			if _, ok := puppetFile.gitModules[comp[1]]; ok {
				return Puppetfile{}, fmt.Errorf("error: Forge Puppet module with same name found in %s for module %s line: %s", pf, comp[1], line)
			}
			// the base url in the Puppetfile takes precedence over an base url specified in the g10k config yaml
			if len(puppetFile.forgeBaseURL) == 0 {
				puppetFile.forgeBaseURL = rt.Config.ForgeBaseURL
			}
			puppetFile.forgeModules[comp[1]] = ForgeModule{version: forgeModuleVersion, name: comp[1], author: comp[0], sha256sum: forgeChecksum, moduleDir: moduleDir, sourceBranch: source + "_" + branch}
		} else if m := reGitModule.FindStringSubmatch(line); len(m) > 1 {
			gitModuleName := m[1]
			//fmt.Println("found git mod name ---> ", gitModuleName)
			if strings.Contains(gitModuleName, "-") {
				rt.Warnf("Warning: Found invalid character '-' in Puppet module name " + gitModuleName + " in " + pf + " line: " + line +
					"\n See module guidelines: https://docs.puppet.com/puppet/latest/reference/lang_reserved.html#modules")
			}
			if len(m[2]) > 1 {
				gitModuleAttributes := m[2]
				//fmt.Println("found git mod attribute ---> ", gitModuleAttributes)
				if strings.Count(gitModuleAttributes, ":git") < 1 && strings.Count(gitModuleAttributes, ":local") < 1 {
					return Puppetfile{}, fmt.Errorf("error: missing :git url in %s for module %s line: %s", pf, gitModuleName, line)
				}
				if strings.Count(gitModuleAttributes, ",") > 3 {
					return Puppetfile{}, fmt.Errorf("error: too many attributes in %s for module %s line: %s", pf, gitModuleName, line)
				}
				if _, ok := puppetFile.gitModules[gitModuleName]; ok {
					return Puppetfile{}, fmt.Errorf("error: duplicate module found in %s for module %s line: %s", pf, gitModuleName, line)
				}
				gas := reUniqueGitAttribute.FindAllStringSubmatch(gitModuleAttributes, -1)
				cga := ""
				if len(gas) > 1 {
					for _, ga := range gas {
						cga += strings.TrimSpace(strings.ReplaceAll(ga[0], "=>", "")) + ", "
					}
					return Puppetfile{}, fmt.Errorf("error: found conflicting git attributes %sin %s for module %s line: %s", cga, pf, gitModuleName, line)
				}
				puppetFile.gitModules[gitModuleName] = GitModule{}
				gm := GitModule{moduleDir: moduleDir}
				gitModuleAttributesArray := strings.Split(gitModuleAttributes, ",")
				//fmt.Println("found git mod attribute array ---> ", gitModuleAttributesArray)
				//fmt.Println("len(gitModuleAttributesArray) --> ", len(gitModuleAttributesArray))
				for i := 0; i <= strings.Count(gitModuleAttributes, ","); i++ {
					//fmt.Println("i -->", i)
					if i >= len(gitModuleAttributesArray) {
						return Puppetfile{}, fmt.Errorf("error: trailing comma or invalid setting for module found in %s for module %s line: %s", pf, gitModuleName, line)
					}
					a := reGitAttribute.FindStringSubmatch(gitModuleAttributesArray[i])
					//fmt.Println("a -->", a)
					if len(a) == 0 {
						return Puppetfile{}, fmt.Errorf("error: trailing comma or invalid setting for module found in %s for module %s line: %s", pf, gitModuleName, line)
					}
					gitModuleAttribute := a[1]
					switch gitModuleAttribute {
					case "git":
						if strings.Contains(a[2], "ProxyCommand") {
							return Puppetfile{}, fmt.Errorf("error: found ProxyCommand option in git url in %s for module %s line: %s", pf, gitModuleName, line)
						}
						gm.git = a[2]
					case "branch":
						if a[2] == ":control_branch" || a[2] == "control_branch" {
							gm.link = true
						} else {
							gm.branch = a[2]
						}
					case "tag":
						gm.tag = a[2]
					case "commit":
						gm.commit = a[2]
					case "ref":
						gm.ref = a[2]
					case "install_path":
						gm.installPath = a[2]
					case "link":
						link, err := strconv.ParseBool(a[2])
						if err != nil {
							return Puppetfile{}, fmt.Errorf("error: can not convert value %s of parameter %s to boolean. In %s for module %s line: %s", a[2], gitModuleAttribute, pf, gitModuleName, line)
						}
						gm.link = link
					case "ignore-unreachable", "ignore_unreachable":
						ignoreUnreachable, err := strconv.ParseBool(a[2])
						if err != nil {
							return Puppetfile{}, fmt.Errorf("error: can not convert value %s of parameter %s to boolean. In %s for module %s line: %s", a[2], gitModuleAttribute, pf, gitModuleName, line)
						}
						gm.ignoreUnreachable = ignoreUnreachable
					case "fallback", "default_branch":
						mapSize := strings.Count(a[2], "|") + 1
						gm.fallback = make([]string, mapSize)
						for i, fallbackBranch := range strings.Split(a[2], "|") {
							//fmt.Println("--------> ", i, strings.TrimSpace(fallbackBranch))
							gm.fallback[i] = strings.TrimSpace(fallbackBranch)
						}
					case "local":
						local, err := strconv.ParseBool(a[2])
						if err != nil {
							return Puppetfile{}, fmt.Errorf("error: can not convert value %s of parameter %s to boolean. In %s for module %s line: %s", a[2], gitModuleAttribute, pf, gitModuleName, line)
						}
						if local {
							gm.local = true
						}
					case "use_ssh_agent":
						useSSHAgent, err := strconv.ParseBool(a[2])
						if err != nil {
							return Puppetfile{}, fmt.Errorf("error: can not convert value %s of parameter %s to boolean. In %s for module %s line: %s", a[2], gitModuleAttribute, pf, gitModuleName, line)
						}
						gm.useSSHAgent = useSSHAgent
					}

				}
				if _, ok := puppetFile.forgeModules[gitModuleName]; ok {
					return Puppetfile{}, fmt.Errorf("error: git puppet module with same name found in %s for module %s line: %s", pf, gitModuleName, line)
				}
				if rt.Config.IgnoreUnreachableModules {
					rt.Debugf("Setting :ignore_unreachable for Git module " + gitModuleName)
					gm.ignoreUnreachable = true
				}
				puppetFile.gitModules[gitModuleName] = gm
			}
		} else {
			// for now only in dry run mode
			if rt.DryRun {
				return Puppetfile{}, fmt.Errorf("error: could not interpret line: %s in %s", line, pf)
			}

		}

	}

	if len(moduleDirs) < 1 {
		// adding at least the default module directory
		moduleDirs = append(moduleDirs, moduleDir)
	}

	if rt.Validate {
		if err := rt.Validatef(); err != nil {
			return Puppetfile{}, err
		}
	}

	puppetFile.moduleDirs = moduleDirs
	puppetFile.sourceBranch = branch
	// fmt.Printf("%+v\n", puppetFile)
	return puppetFile, nil
}
