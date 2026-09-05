package g10k

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"golang.org/x/sys/unix"
)

var ErrValidationFailed = errors.New("config validation failed")
var ErrFatal = errors.New("g10k: fatal error")

// Debugf is a helper function for debug logging if global variable debug is set to true
func (rt *Runtime) Debugf(s string) {
	if rt.Debug {
		pc, _, _, _ := runtime.Caller(1)
		callingFunctionName := strings.Split(runtime.FuncForPC(pc).Name(), ".")[len(strings.Split(runtime.FuncForPC(pc).Name(), "."))-1]
		if strings.HasPrefix(callingFunctionName, "func") {
			// check for anonymous function names
			log.Print("DEBUG " + fmt.Sprint(s))
		} else {
			log.Print("DEBUG " + callingFunctionName + "(): " + fmt.Sprint(s))
		}
	}
}

// Verbosef is a helper function for verbose logging if global variable verbose is set to true
func (rt *Runtime) Verbosef(s string) {
	if rt.Debug || rt.Verbose {
		log.Print(fmt.Sprint(s))
	}
}

// Infof is a helper function for info logging if global variable info is set to true
func (rt *Runtime) Infof(s string) {
	if rt.Debug || rt.Verbose || rt.Info {
		color.Green(s)
	}
}

// Validatef is a helper function for validation logging if global variable validate is set to true
func (rt *Runtime) Validatef() error {
	if len(rt.ValidationMessages) > 0 {
		for _, message := range rt.ValidationMessages {
			_, _ = color.New(color.FgRed).Fprintln(os.Stdout, message) // FIXME: error should be handled
		}
		return ErrValidationFailed
	}
	_, _ = color.New(color.FgGreen).Fprintln(os.Stdout, "Configuration successfully parsed.") // FIXME: error should be handled
	return nil
}

// Warnf is a helper function for warning logging
func (rt *Runtime) Warnf(s string) {
	color.Set(color.FgYellow)
	fmt.Println(s)
	color.Unset()
}

// Fatalf is a helper function for fatal logging
func (rt *Runtime) Fatalf(s string) {
	if rt.Validate {
		rt.ValidationMessages = append(rt.ValidationMessages, s)
	} else {
		_, _ = color.New(color.FgRed).Fprintln(os.Stderr, s) // FIXME: error should be handled
		os.Exit(1)
	}
}

// fileExists checks if the given file exists and returns a bool
func fileExists(file string) bool {
	//Debugf("checking for file existence " + file)
	if _, err := os.Lstat(file); os.IsNotExist(err) {
		return false
	}
	return true
}

// isDir checks if the given dir exists and returns a bool
func isDir(dir string) bool {
	fi, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return false
	}
	if fi.Mode().IsDir() {
		return true
	}
	return false
}

// normalizeDir removes from the given directory path multiple redundant slashes and removes a trailing slash
func normalizeDir(dir string) string {
	if strings.Count(dir, "//") > 0 {
		dir = normalizeDir(strings.ReplaceAll(dir, "//", "/"))
	}
	dir = strings.TrimSuffix(dir, "/")
	return dir
}

// checkDirAndCreate tests if the given directory exists and tries to create it
func (rt *Runtime) checkDirAndCreate(dir string, name string) string {
	if !rt.DryRun {
		if len(dir) != 0 {
			if !fileExists(dir) {
				//log.Printf("checkDirAndCreate(): trying to create dir '%s' as %s", dir, name){
				if err := os.MkdirAll(dir, 0777); err != nil {
					rt.Fatalf("checkDirAndCreate(): Error: failed to create directory: " + dir)
				}
			} else {
				if !isDir(dir) {
					rt.Fatalf("checkDirAndCreate(): Error: " + dir + " exists, but is not a directory! Exiting!")
				} else {
					if unix.Access(dir, unix.W_OK) != nil {
						rt.Fatalf("checkDirAndCreate(): Error: " + dir + " exists, but is not writable! Exiting!")
					}
				}
			}
		} else {
			// TODO make dir optional
			rt.Fatalf("checkDirAndCreate(): Error: dir setting '" + name + "' missing! Exiting!")
		}
	}
	dir = normalizeDir(dir)
	rt.Debugf("Using as " + name + ": " + dir)
	return dir
}

func (rt *Runtime) createOrPurgeDir(dir string, callingFunction string) {
	if !rt.DryRun {
		if !fileExists(dir) {
			rt.Debugf("Trying to create dir: " + dir + " called from " + callingFunction)
			_ = os.MkdirAll(dir, 0777) // FIXME: error should be handled
		} else {
			rt.Debugf("Trying to remove: " + dir + " called from " + callingFunction)
			if err := os.RemoveAll(dir); err != nil {
				log.Print("createOrPurgeDir(): error: removing dir failed", err)
			}
			rt.Debugf("Trying to create dir: " + dir + " called from " + callingFunction)
			_ = os.MkdirAll(dir, 0777) // FIXME: error should be handled
		}
	}
}

func (rt *Runtime) purgeDir(dir string, callingFunction string) {
	if !fileExists(dir) {
		rt.Debugf("Unnecessary to remove dir: " + dir + " it does not exist. Called from " + callingFunction)
	} else {
		rt.Debugf("Trying to remove: " + dir + " called from " + callingFunction)
		if err := os.RemoveAll(dir); err != nil {
			log.Print("purgeDir(): os.RemoveAll() error: removing dir failed: ", err.Error())
			if err = syscall.Unlink(dir); err != nil {
				log.Print("purgeDir(): syscall.Unlink() error: removing link failed: ", err.Error())
			}
		}
	}
}

func (rt *Runtime) executeCommand(command string, commandDir string, timeout int, allowFail bool, disableHttpProxy bool) ExecResult {
	if len(commandDir) > 0 {
		rt.Debugf("Executing " + command + " in cwd " + commandDir)
	} else {
		rt.Debugf("Executing " + command)
	}
	parts := strings.SplitN(command, " ", 2)
	cmd := parts[0]
	cmdArgs := []string{}
	if len(parts) > 1 {
		args, err := splitCommandLine(parts[1])
		if err != nil {
			rt.Debugf("err: " + fmt.Sprint(err))
		} else {
			cmdArgs = args
		}
	}

	before := time.Now()
	execCommand := exec.Command(cmd, cmdArgs...)
	if len(commandDir) > 0 {
		execCommand.Dir = commandDir
	}
	if disableHttpProxy {
		rt.Debugf("found matching NO_PROXY URL, trying to disable http_proxy and https_proxy env variables for " + command)
		// execCommand.Env = append(os.Environ(), "http_proxy=")
		// execCommand.Env = append(os.Environ(), "https_proxy=")
		_ = os.Unsetenv("http_proxy")  // FIXME: error should be handled
		_ = os.Unsetenv("https_proxy") // FIXME: error should be handled
		_ = os.Unsetenv("HTTP_PROXY")  // FIXME: error should be handled
		_ = os.Unsetenv("HTTPS_PROXY") // FIXME: error should be handled
	}
	execCommand.Env = os.Environ()
	out, err := execCommand.CombinedOutput()
	duration := time.Since(before).Seconds()
	er := ExecResult{0, string(out)}
	if msg, ok := err.(*exec.ExitError); ok { // there is error code
		er.returnCode = msg.Sys().(syscall.WaitStatus).ExitStatus()
	}
	if (allowFail || rt.Config.UseCacheFallback) && err != nil {
		rt.Debugf("Executing " + command + " took " + strconv.FormatFloat(duration, 'f', 5, 64) + "s")
	} else {
		rt.Verbosef("Executing " + command + " took " + strconv.FormatFloat(duration, 'f', 5, 64) + "s")
	}
	if err != nil {
		er.returnCode = 1
		er.output = fmt.Sprint(err) + " " + fmt.Sprint(string(out))
	}
	return er
}

func splitCommandLine(input string) ([]string, error) {
	var words []string
	var word strings.Builder
	inWord := false
	quote := rune(0)
	escaped := false

	flush := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord = false
		}
	}

	for _, character := range input {
		if escaped {
			if character == '\n' {
				escaped = false
				continue
			}
			if quote == '"' && !strings.ContainsRune("$`\"\\", character) {
				word.WriteRune('\\')
			}
			word.WriteRune(character)
			escaped = false
			inWord = true
			continue
		}

		if quote == '\'' {
			if character == '\'' {
				quote = 0
			} else {
				word.WriteRune(character)
			}
			inWord = true
			continue
		}

		if quote == '"' {
			switch character {
			case '"':
				quote = 0
			case '\\':
				escaped = true
			default:
				word.WriteRune(character)
			}
			inWord = true
			continue
		}

		switch character {
		case '\\':
			escaped = true
		case '\'', '"':
			quote = character
			inWord = true
		case ' ', '\n', '\t':
			flush()
		default:
			word.WriteRune(character)
			inWord = true
		}
	}

	if escaped {
		return nil, fmt.Errorf("unterminated backslash escape")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quoted string")
	}
	flush()
	return words, nil
}

// funcName return the function name as a string
func funcName() string {
	pc, _, _, _ := runtime.Caller(1)
	completeFuncname := runtime.FuncForPC(pc).Name()
	return strings.Split(completeFuncname, ".")[len(strings.Split(completeFuncname, "."))-1]
}

func (rt *Runtime) timeTrack(start time.Time, name string) {
	duration := time.Since(start).Seconds()
	switch name {
	case "resolveForgeModules":
		rt.SyncForgeTime = duration
	case "resolveGitRepositories":
		rt.SyncGitTime = duration
	}
	rt.Debugf(name + "() took " + strconv.FormatFloat(duration, 'f', 5, 64) + "s")
}

// checkForAndExecutePostrunCommand check if a `postrun` command was specified in the g10k config and executes it
func (rt *Runtime) checkForAndExecutePostrunCommand() {
	if len(rt.Config.PostRunCommand) > 0 {
		postrunCommandString := strings.Join(rt.Config.PostRunCommand, " ")
		postrunCommandString = strings.ReplaceAll(postrunCommandString, "$modifieddirs", strings.Join(rt.NeedSyncDirs, " "))

		needSyncEnvText := ""
		for needSyncEnv := range rt.NeedSyncEnvs {
			needSyncEnvText += needSyncEnv + " "
		}
		postrunCommandString = strings.ReplaceAll(postrunCommandString, "$modifiedenvs", needSyncEnvText)
		postrunCommandString = strings.ReplaceAll(postrunCommandString, "$branchparam", rt.Branch)

		er := rt.executeCommand(postrunCommandString, "", rt.Config.Timeout, false, false)
		rt.Debugf("postrun command '" + postrunCommandString + "' terminated with exit code " + strconv.Itoa(er.returnCode))
	}
}

// getSha256sumFile return the SHA256 hash sum of the given file
func (rt *Runtime) getSha256sumFile(file string) string {
	// https://golang.org/pkg/crypto/sha256/#New
	f, err := os.Open(file)
	if err != nil {
		rt.Fatalf("failed to open file " + file + " to calculate SHA256 sum. Error: " + err.Error())
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		rt.Fatalf("failed to calculate SHA256 sum of file " + file + " Error: " + err.Error())
	}

	return hex.EncodeToString(h.Sum(nil))
}

// moveFile uses io.Copy to create a copy of the given file https://stackoverflow.com/a/50741908/682847
func (rt *Runtime) moveFile(sourcePath, destPath string, deleteSourceFileToggle bool) error {
	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("couldn't open source file: %s", err)
	}
	outputFile, err := os.Create(destPath)
	if err != nil {
		_ = inputFile.Close()
		return fmt.Errorf("couldn't open dest file: %s", err)
	}
	defer func() { _ = outputFile.Close() }()
	_, err = io.Copy(outputFile, inputFile)
	_ = inputFile.Close()
	if err != nil {
		return fmt.Errorf("writing to output file failed: %s", err)
	}
	if deleteSourceFileToggle {
		// The copy was successful, so now delete the original file
		err = os.Remove(sourcePath)
		if err != nil {
			return fmt.Errorf("failed removing original file: %s", err)
		}
	}
	return nil
}

func stringSliceContains(slice []string, element string) bool {
	for _, e := range slice {
		if e == element {
			return true
		}
	}
	return false
}

func (rt *Runtime) writeStructJSONFile(file string, v interface{}) {
	content, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		rt.Warnf("Could not encode JSON file " + file + " " + err.Error())
	}

	err = os.WriteFile(file, content, 0644)
	if err != nil {
		rt.Warnf("Could not write JSON file " + file + " " + err.Error())
	}

}

func (rt *Runtime) readDeployResultFile(file string) DeployResult {
	// Open our jsonFile
	jsonFile, err := os.Open(file)
	// if we os.Open returns an error then handle it
	if err != nil {
		rt.Warnf("Could not open JSON file " + file + " " + err.Error())
	}
	defer func() {
		_ = jsonFile.Close()
	}()

	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		rt.Warnf("Could not read JSON file " + file + " " + err.Error())
	}

	var dr DeployResult
	_ = json.Unmarshal([]byte(byteValue), &dr) // FIXME an error here should actually be handled

	return dr

}

func stripComponent(component string, env string) string {
	if regexp.MustCompile(`^/.*/$`).MatchString(component) {
		return regexp.MustCompile(component[1:len(component)-1]).ReplaceAllString(env, "")
	} else {
		return strings.TrimPrefix(env, component)
	}
}

func (rt *Runtime) matchGitRemoteURLNoProxy(url string) bool {
	noProxy := os.Getenv("NO_PROXY")
	for _, np := range strings.Split(noProxy, ",") {
		if len(np) > 0 {
			if strings.Contains(url, np) {
				rt.Debugf("found NO_PROXY setting: " + np + " matching  " + url)
				return true
			}
		}
	}
	// do the same for lower case environment variable name
	noProxyL := os.Getenv("no_proxy")
	for _, np := range strings.Split(noProxyL, ",") {
		if len(np) > 0 {
			if strings.Contains(url, np) {
				rt.Debugf("found no_proxy setting: " + np + " matching  " + url)
				return true
			}
		}
	}
	return false
}
