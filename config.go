package main

import (
	"bufio"
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
func readConfigfile(configFile string) ConfigSettings {
	Debugf("Trying to read g10k config file: " + configFile)
	data, err := os.ReadFile(configFile)
	if err != nil {
		Fatalf("readConfigfile(): There was an error parsing the config file " + configFile + ": " + err.Error())
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
		Fatalf("YAML unmarshal error: " + err.Error())
	}

	if len(os.Getenv("g10k_cachedir")) > 0 {
		cachedir := os.Getenv("g10k_cachedir")
		Debugf("Found environment variable g10k_cachedir set to: " + cachedir)
		config.CacheDir = checkDirAndCreate(cachedir, "cachedir environment variable g10k_cachedir")
	} else {
		config.CacheDir = checkDirAndCreate(config.CacheDir, "cachedir from g10k config "+configFile)
	}

	config.CacheDir = checkDirAndCreate(config.CacheDir, "cachedir")
	config.ForgeCacheDir = checkDirAndCreate(filepath.Join(config.CacheDir, "forge"), "cachedir/forge")
	config.ModulesCacheDir = checkDirAndCreate(filepath.Join(config.CacheDir, "modules"), "cachedir/modules")
	config.EnvCacheDir = checkDirAndCreate(filepath.Join(config.CacheDir, "environments"), "cachedir/environments")

	if len(config.ForgeBaseURL) == 0 {
		config.ForgeBaseURL = "https://forgeapi.puppet.com"
	}

	// fmt.Println("Forge Baseurl: ", config.ForgeBaseURL)

	// set default timeout to 5 seconds if no timeout setting found
	if config.Timeout == 0 {
		config.Timeout = 5
	}

	if usecacheFallback {
		config.UseCacheFallback = true
	}

	if retryGitCommands {
		config.RetryGitCommands = true
	}

	if gitObjectSyntaxNotSupported {
		config.GitObjectSyntaxNotSupported = true
	}

	// set default max Go routines for Forge and Git module resolution if none is given
	if config.Maxworker <= 0 {
		config.Maxworker = maxworker
	}
	if maxworker != 50 {
		config.Maxworker = maxworker
	}

	if maxworker == 0 && config.Maxworker == 0 {
		config.Maxworker = 50
	}

	// set default max Go routines for Forge and Git module extracting
	if config.MaxExtractworker <= 0 {
		config.MaxExtractworker = maxExtractworker
	}
	if maxExtractworker != 20 {
		config.MaxExtractworker = maxExtractworker
	}

	if maxExtractworker == 0 && config.MaxExtractworker == 0 {
		config.MaxExtractworker = 20
	}

	if len(config.ForgeCacheTTLString) != 0 {
		ttl, err := time.ParseDuration(config.ForgeCacheTTLString)
		if err != nil {
			Fatalf("Error: Can not convert value " + config.ForgeCacheTTLString + " of config setting forge_cache_ttl to a golang Duration. Valid time units are 300ms, 1.5h or 2h45m. In " + configFile)
		}
		config.ForgeCacheTTL = ttl
	}

	// check for non-empty config.Deploy which takes precedence over the non-deploy scoped settings
	// See https://github.com/puppetlabs/r10k/blob/master/doc/dynamic-environments/configuration.mkd#deploy
	emptyDeploy := DeploySettings{}
	if !reflect.DeepEqual(config.Deploy, emptyDeploy) {
		Debugf("detected deploy configuration hash, which takes precedence over the non-deploy scoped settings")
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

	if validate {
		Validatef()
	}

	// fmt.Printf("%+v\n", config)
	return config
}

// preparePuppetfile remove whitespace and comment lines from the given Puppetfile and merges Puppetfile resources that are identified with having a , at the end
func preparePuppetfile(pf string) string {
	file, err := os.Open(pf)
	if err != nil {
		Fatalf("preparePuppetfile(): Error while opening Puppetfile " + pf + " Error: " + err.Error())
	}
	defer func() {
		_ = file.Close()
	}()

	reComma := regexp.MustCompile(`,\s*$`)
	reComment := regexp.MustCompile(`^\s*#`)
	reEmpty := regexp.MustCompile("^$")

	pfString := ""
	scanner := bufio.NewScanner(file)
	Debugf("scanning file: " + pf)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !reComment.MatchString(line) && !reEmpty.MatchString(line) {
			if strings.Contains(line, "#") {
				Debugf("found inline comment in " + pf + "line: " + line)
				line = strings.Split(line, "#")[0]
			}
			if reComma.MatchString(line) {
				pfString += line
				Debugf("adding line:" + line)
			} else {
				pfString += line + "\n"
				Debugf("adding line:" + line)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		Fatalf("preparePuppetfile(): Error while scanning Puppetfile " + pf + " Error: " + err.Error())
	}

	return pfString
}

// readPuppetfile creates the ConfigSettings struct from the Puppetfile
func readPuppetfile(pf string, sshKey string, source string, branch string, forceForgeVersions bool, replacedPuppetfileContent bool) Puppetfile {
	var puppetFile Puppetfile
	var n string
	puppetFile.privateKey = sshKey
	puppetFile.source = source
	puppetFile.forgeModules = map[string]ForgeModule{}
	puppetFile.gitModules = map[string]GitModule{}
	if replacedPuppetfileContent {
		Debugf("Using replaced Puppetfile content, probably because a Git module was found in Forge notation")
		n = pf
	} else {
		Debugf("Trying to parse: " + pf)
		n = preparePuppetfile(pf)
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
	if len(moduleDirParam) != 0 {
		moduleDir = moduleDirParam
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
			Fatalf("Error: trailing comma found in " + pf + " somewhere here: " + line)
		}
		if m := reDanglingAttribute.FindStringSubmatch(line); len(m) >= 1 {
			previousLine := ""
			if i-1 >= 0 {
				previousLine = lines[i-1]
			}
			Fatalf("Error: found dangling module attribute in " + pf + " somewhere here: " + previousLine + line + " Check for missing , at the end of the line.")
		}
		if m := reModuledir.FindStringSubmatch(line); len(m) > 1 && len(moduleDirParam) == 0 {
			moduleDir = normalizeDir(m[1])
			moduleDirs = append(moduleDirs, moduleDir)
		} else if m := reForgeBaseURL.FindStringSubmatch(line); len(m) > 1 {
			puppetFile.forgeBaseURL = m[1]
			//fmt.Println("found forge base URL parameter ---> ", m[1])
		} else if m := reForgeCacheTTL.FindStringSubmatch(line); len(m) > 1 {
			ttl, err := time.ParseDuration(m[1])
			if err != nil {
				Fatalf("Error: Can not convert value " + m[1] + " of parameter " + m[0] + " to a golang Duration. Valid time units are 300ms, 1.5h or 2h45m. In " + pf + " line: " + line)
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
					Fatalf("Error: Forge module name is invalid! Should be like puppetlabs/apt or puppetlabs-apt, but is: " + m[2] + " in " + pf + " line: " + line)
				}
			}
			forgeModuleName = comp[0] + "/" + comp[1]
			if _, ok := puppetFile.forgeModules[comp[1]]; ok {
				Fatalf("Error: Duplicate forge module found in " + pf + " for module " + forgeModuleName + " line: " + line)
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
						Debugf("setting forge module " + forgeModuleName + " to version " + forgeModuleVersion)
					}
					if len(a[2]) > 1 {
						//fmt.Println("a[2] ---> ", a[2])
						forgeAttributeName := strings.TrimSpace(a[1])
						forgeAttributeValue := strings.TrimSpace(a[2])
						Debugf("found forge attribute ---> " + forgeAttributeName + " with value ---> " + forgeAttributeValue)
						if forgeAttributeName == ":sha256sum" {
							forgeChecksum = forgeAttributeValue
						} else if forgeAttribute == "git" {
							// try to detect Git modules in Forge <AUTHOR>/<MODULENAME> notation, fixes #104
							Debugf("Found git module in Forge notation: " + forgeModuleName + " with git url: " + forgeAttributeValue)
							//fmt.Println("line:", line)
							removeForgeNotationAuthor := strings.Split(line, forgeModuleNameSeparator)
							if len(removeForgeNotationAuthor) < 2 {
								Fatalf("Error: Found git module in Forge notation: " + forgeModuleName + " with git url: " + forgeAttributeValue + ", but something went wrong while trying to remove the author part to make g10k detect it as an Git module module:" + comp[1] + " line: " + line)
							} else {
								//fmt.Println("removeForgeNotationAuthor:", removeForgeNotationAuthor[0])
								replacedLine := strings.Replace(line, removeForgeNotationAuthor[0]+forgeModuleNameSeparator, "mod '", 1)
								//fmt.Println("replacedLine:", replacedLine)
								//fmt.Print("n:", n)
								newN := strings.Replace(n, line, replacedLine, 1)
								//fmt.Print("newN:", newN)
								return readPuppetfile(newN, sshKey, source, branch, forceForgeVersions, true)
							}
						}
					}
				}
			}
			if forceForgeVersions && (forgeModuleVersion == "present" || forgeModuleVersion == "latest") {
				Fatalf("Error: Found " + forgeModuleVersion + " setting for forge module in " + pf + " for module " + forgeModuleName + " line: " + line + " and force_forge_versions is set to true! Please specify a version (e.g. '2.3.0')")
			}
			if _, ok := puppetFile.gitModules[comp[1]]; ok {
				Fatalf("Error: Forge Puppet module with same name found in " + pf + " for module " + comp[1] + " line: " + line)
			}
			// the base url in the Puppetfile takes precedence over an base url specified in the g10k config yaml
			if len(puppetFile.forgeBaseURL) == 0 {
				puppetFile.forgeBaseURL = config.ForgeBaseURL
			}
			puppetFile.forgeModules[comp[1]] = ForgeModule{version: forgeModuleVersion, name: comp[1], author: comp[0], sha256sum: forgeChecksum, moduleDir: moduleDir, sourceBranch: source + "_" + branch}
		} else if m := reGitModule.FindStringSubmatch(line); len(m) > 1 {
			gitModuleName := m[1]
			//fmt.Println("found git mod name ---> ", gitModuleName)
			if strings.Contains(gitModuleName, "-") {
				Warnf("Warning: Found invalid character '-' in Puppet module name " + gitModuleName + " in " + pf + " line: " + line +
					"\n See module guidelines: https://docs.puppet.com/puppet/latest/reference/lang_reserved.html#modules")
			}
			if len(m[2]) > 1 {
				gitModuleAttributes := m[2]
				//fmt.Println("found git mod attribute ---> ", gitModuleAttributes)
				if strings.Count(gitModuleAttributes, ":git") < 1 && strings.Count(gitModuleAttributes, ":local") < 1 {
					Fatalf("Error: Missing :git url in " + pf + " for module " + gitModuleName + " line: " + line)
				}
				if strings.Count(gitModuleAttributes, ",") > 3 {
					Fatalf("Error: Too many attributes in " + pf + " for module " + gitModuleName + " line: " + line)
				}
				if _, ok := puppetFile.gitModules[gitModuleName]; ok {
					Fatalf("Error: Duplicate module found in " + pf + " for module " + gitModuleName + " line: " + line)
				}
				gas := reUniqueGitAttribute.FindAllStringSubmatch(gitModuleAttributes, -1)
				cga := ""
				if len(gas) > 1 {
					for _, ga := range gas {
						cga += strings.TrimSpace(strings.ReplaceAll(ga[0], "=>", "")) + ", "
					}
					Fatalf("Error: Found conflicting git attributes " + cga + "in " + pf + " for module " + gitModuleName + " line: " + line)
				}
				puppetFile.gitModules[gitModuleName] = GitModule{}
				gm := GitModule{moduleDir: moduleDir}
				gitModuleAttributesArray := strings.Split(gitModuleAttributes, ",")
				//fmt.Println("found git mod attribute array ---> ", gitModuleAttributesArray)
				//fmt.Println("len(gitModuleAttributesArray) --> ", len(gitModuleAttributesArray))
				for i := 0; i <= strings.Count(gitModuleAttributes, ","); i++ {
					//fmt.Println("i -->", i)
					if i >= len(gitModuleAttributesArray) {
						Fatalf("Error: Trailing comma or invalid setting for module found in " + pf + " for module " + gitModuleName + " line: " + line)
					}
					a := reGitAttribute.FindStringSubmatch(gitModuleAttributesArray[i])
					//fmt.Println("a -->", a)
					if len(a) == 0 {
						Fatalf("Error: Trailing comma or invalid setting for module found in " + pf + " for module " + gitModuleName + " line: " + line)
					}
					gitModuleAttribute := a[1]
					switch gitModuleAttribute {
					case "git":
						if strings.Contains(a[2], "ProxyCommand") {
							Fatalf("Error: Found ProxyCommand option in git url in " + pf + " for module " + gitModuleName + " line: " + line)
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
							Fatalf("Error: Can not convert value " + a[2] + " of parameter " + gitModuleAttribute + " to boolean. In " + pf + " for module " + gitModuleName + " line: " + line)
						}
						gm.link = link
					case "ignore-unreachable", "ignore_unreachable":
						ignoreUnreachable, err := strconv.ParseBool(a[2])
						if err != nil {
							Fatalf("Error: Can not convert value " + a[2] + " of parameter " + gitModuleAttribute + " to boolean. In " + pf + " for module " + gitModuleName + " line: " + line)
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
							Fatalf("Error: Can not convert value " + a[2] + " of parameter " + gitModuleAttribute + " to boolean. In " + pf + " for module " + gitModuleName + " line: " + line)
						}
						if local {
							gm.local = true
						}
					case "use_ssh_agent":
						useSSHAgent, err := strconv.ParseBool(a[2])
						if err != nil {
							Fatalf("Error: Can not convert value " + a[2] + " of parameter " + gitModuleAttribute + " to boolean. In " + pf + " for module " + gitModuleName + " line: " + line)
						}
						gm.useSSHAgent = useSSHAgent
					}

				}
				if _, ok := puppetFile.forgeModules[gitModuleName]; ok {
					Fatalf("Error: Git Puppet module with same name found in " + pf + " for module " + gitModuleName + " line: " + line)
				}
				if config.IgnoreUnreachableModules {
					Debugf("Setting :ignore_unreachable for Git module " + gitModuleName)
					gm.ignoreUnreachable = true
				}
				puppetFile.gitModules[gitModuleName] = gm
			}
		} else {
			// for now only in dry run mode
			if dryRun {
				Fatalf("Error: Could not interpret line: " + line + " In " + pf)
			}

		}

	}

	if len(moduleDirs) < 1 {
		// adding at least the default module directory
		moduleDirs = append(moduleDirs, moduleDir)
	}

	if validate {
		Validatef()
	}

	puppetFile.moduleDirs = moduleDirs
	puppetFile.sourceBranch = branch
	// fmt.Printf("%+v\n", puppetFile)
	return puppetFile
}
