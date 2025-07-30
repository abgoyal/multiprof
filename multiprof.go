package main

import (
	// the initial _ needs to be there,
	// otherwise we get "embed imported and not used"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	"github.com/BurntSushi/toml"
	"github.com/gobwas/glob"
)

// --- Embedded Content ---
//
//go:embed help.txt
var helpText string

//go:embed default.toml
var defaultConfigToml string

//go:embed completion.template.bash
var completionTemplate string

//go:embed init.txt
var initHelpText string

// --- Constants ---
const (
	configDirName     = ".config/multiprof"
	configFileName    = "config.toml"
	wrapperDirName    = ".local/bin/multiprof"
	completionDirName = ".local/share/bash-completion/completions"
	debugEnvVar       = "MULTIPROF_DEBUG"
)

// --- Global State ---
var debugMode bool

func init() {
	debugMode = os.Getenv(debugEnvVar) == "1" || os.Getenv(debugEnvVar) == "true"
}

// --- Configuration Structs ---
type Config struct {
	Settings Settings `toml:"settings"`
	Rules    []Rule   `toml:"rules"`
}
type Settings struct {
	Suffix string `toml:"suffix"`
}
type Rule struct {
	Pattern string `toml:"pattern"`
	Home    string `toml:"home"`
}

// --- Main Logic ---

func main() {
	log.SetFlags(0)
	ownExecutable, err := os.Executable()
	if err != nil {
		logError("Critical error: Cannot determine own path: %v", err)
		os.Exit(1)
	}
	calledAs := filepath.Base(os.Args[0])
	ownName := filepath.Base(ownExecutable)

	if calledAs == ownName || calledAs == "main" { // for go run
		if len(os.Args) < 2 {
			printUsage()
			return
		}
		runManager(os.Args[1], os.Args[2:])
	} else {
		runWrapper()
	}
}

func runManager(command string, args []string) {
	switch command {
	case "init":
		runInit()
	case "add-rule":
		runAddRule(args)
	case "add-wrapper":
		runAddWrapper(args)
	case "list":
		runList()
	case "help", "-h", "--help":
		printUsage()
	default:
		logError("Unknown command '%s'. Run 'multiprof help' for a list of commands.", command)
		os.Exit(1)
	}
}

// --- Wrapper Execution ---

func runWrapper() {
	config, _ := loadConfig()
	cwd, _ := os.Getwd()
	expandedCwd := expandPath(cwd)
	expandedCwdWithSlash := expandedCwd + string(os.PathSeparator)
	debugf("Checking match for '%s' and '%s'", expandedCwd, expandedCwdWithSlash)

	var newHome string
	profileMatched := false
	for _, rule := range config.Rules {
		expandedPattern := expandPath(rule.Pattern)
		g, _ := glob.Compile(expandedPattern)
		if g.Match(expandedCwd) || g.Match(expandedCwdWithSlash) {
			debugf("Matched Rule with pattern: '%s'", rule.Pattern)
			newHome = expandPath(rule.Home)
			profileMatched = true
			break
		}
	}

	if !profileMatched {
		logError("No multiprof Rule matched the current directory: %s", cwd)
		logInfo("To add a Rule, run: multiprof add-rule --pattern \"%s/**\" --home \"/path/to/home\"", cwd)
		os.Exit(1)
	}
	os.Setenv("HOME", newHome)
	debugf("Set HOME to: '%s'", newHome)

	wrapperName := filepath.Base(os.Args[0])
	targetCmdName := strings.TrimSuffix(wrapperName, config.Settings.Suffix)
	originalPath := os.Getenv("PATH")
	wrapperDir, _ := getWrapperDir()
	safePath := strings.ReplaceAll(originalPath, wrapperDir+":", "")
	os.Setenv("PATH", safePath)
	debugf("Temporarily searching for '%s' in safe PATH", targetCmdName)

	targetCmdPath, err := exec.LookPath(targetCmdName)
	os.Setenv("PATH", originalPath)

	if err != nil {
		logError("Could not find target command '%s' in the system PATH: %v", targetCmdName, err)
		os.Exit(1)
	}
	debugf("Executing: %s", targetCmdPath)
	syscall.Exec(targetCmdPath, os.Args, os.Environ())
}

// --- Management Commands ---

func runInit() {
	logInfo("Running setup wizard...")
	createDefaultConfig()
	logSuccess("Ensured config file exists at ~/.config/multiprof/config.toml")
	wrapperDir, _ := getWrapperDir()
	os.MkdirAll(wrapperDir, 0755)
	logSuccess("Ensured Wrapper Directory exists at ~/" + strings.TrimPrefix(wrapperDir, os.Getenv("HOME")+"/"))

	tmpl, err := template.New("init").Parse(initHelpText)
	if err != nil {
		logError("Could not parse init template: %v", err)
		return
	}
	data := struct{ WrapperDir string }{WrapperDir: wrapperDir}
	tmpl.Execute(os.Stdout, data)
}

func runAddRule(args []string) {
	addCmd := flag.NewFlagSet("add-rule", flag.ExitOnError)
	patternFlag := addCmd.String("pattern", "", "Glob pattern to match a directory context.")
	homeFlag := addCmd.String("home", "", "The directory to use as $HOME when the pattern matches.")
	addCmd.Parse(args)
	if *patternFlag == "" || *homeFlag == "" {
		logError("--pattern and --home flags are required.")
		addCmd.Usage()
		os.Exit(1)
	}
	config, _ := loadConfig()
	newPattern := expandPath(*patternFlag)
	for _, rule := range config.Rules {
		existingPattern := expandPath(rule.Pattern)
		g, _ := glob.Compile(existingPattern)
		if g.Match(newPattern) {
			logWarn("New pattern '%s' may be shadowed by existing Rule '%s'.", *patternFlag, rule.Pattern)
			logInfo("Rule priority is determined by their order in the config file.")
			break
		}
	}
	config.Rules = append(config.Rules, Rule{Pattern: *patternFlag, Home: *homeFlag})
	saveConfig(config)
	logSuccess("Added Rule: when in '%s', use '%s' as HOME.", *patternFlag, *homeFlag)
}

func runAddWrapper(args []string) {
	if len(args) != 1 {
		logError("Usage: multiprof add-wrapper <command_name>")
		os.Exit(1)
	}
	cmdName := args[0]
	config, _ := loadConfig()

	wrapperName := cmdName + config.Settings.Suffix
	wrapperDir, _ := getWrapperDir()
	if !strings.Contains(os.Getenv("PATH"), wrapperDir) {
		logWarn("Wrapper Directory '%s' not found in your $PATH.", wrapperDir)
		logInfo("Please run `multiprof init` and follow the setup instructions.")
	}

	multiprofPath, _ := os.Executable()
	symlinkPath := filepath.Join(wrapperDir, wrapperName)
	if err := os.Symlink(multiprofPath, symlinkPath); err != nil {
		if !os.IsExist(err) {
			logError("Failed to create Wrapper: %v", err)
			os.Exit(1)
		}
	}
	logSuccess("Created Wrapper for '%s' at %s", cmdName, symlinkPath)

	if config.Settings.Suffix != "" {
		if err := createCompletionFile(wrapperName, cmdName); err != nil {
			logWarn("Could not create completion file: %v", err)
		} else {
			logSuccess("Created completion file for '%s'.", wrapperName)
		}
	}

}

func createCompletionFile(wrapperName, originalCmd string) error {
	completionDir, err := getCompletionDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(completionDir, 0755); err != nil {
		return fmt.Errorf("could not create completion directory: %w", err)
	}

	completionFilePath := filepath.Join(completionDir, wrapperName)
	f, err := os.Create(completionFilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	tmpl, err := template.New("completion").Parse(completionTemplate)
	if err != nil {
		return err
	}

	data := struct {
		WrapperName string
		OriginalCmd string
		HookName    string
	}{
		WrapperName: wrapperName,
		OriginalCmd: originalCmd,
		HookName:    "multiprof_hook_" + strings.ReplaceAll(wrapperName, "-", "_"),
	}

	return tmpl.Execute(f, data)
}

func printUsage() {
	fmt.Print(helpText)
}

func runList() {
	config, _ := loadConfig()
	fmt.Printf("Wrapper Suffix: \"%s\"\n", config.Settings.Suffix)
	fmt.Println("--- Rules (checked in order of priority) ---")
	if len(config.Rules) == 0 {
		fmt.Println("No Rules defined. Use 'multiprof add-rule' to create one.")
		return
	}
	for i, rule := range config.Rules {
		fmt.Printf("%d: When in '%s', use '%s' as HOME.\n", i+1, rule.Pattern, rule.Home)
	}
}

// --- Helpers ---
func logInfo(format string, v ...interface{})    { fmt.Printf("[INFO] "+format+"\n", v...) }
func logSuccess(format string, v ...interface{}) { fmt.Printf("[OK] "+format+"\n", v...) }
func logWarn(format string, v ...interface{})    { fmt.Printf("[WARN] "+format+"\n", v...) }
func logError(format string, v ...interface{})   { fmt.Fprintf(os.Stderr, "[FAIL] "+format+"\n", v...) }
func debugf(format string, v ...interface{}) {
	if debugMode {
		log.Printf("[DEBUG] "+format, v...)
	}
}
func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(homeDir, path[1:])
		}
	}
	return os.ExpandEnv(path)
}
func getWrapperDir() (string, error) { return expandPath(filepath.Join("~/", wrapperDirName)), nil }
func getCompletionDir() (string, error) {
	return expandPath(filepath.Join("~/", completionDirName)), nil
}
func getConfigPath() (string, error) {
	return expandPath(filepath.Join("~/", configDirName, configFileName)), nil
}
func createDefaultConfig() error {
	configPath, _ := getConfigPath()
	if _, err := os.Stat(configPath); err == nil {
		return nil // File already exists
	}
	os.MkdirAll(filepath.Dir(configPath), 0755)
	return os.WriteFile(configPath, []byte(defaultConfigToml), 0644)
}
func loadConfig() (Config, error) {
	var config Config
	configPath, _ := getConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		createDefaultConfig()
	}
	if _, err := toml.DecodeFile(configPath, &config); err != nil {
		return config, err
	}
	return config, nil
}
func saveConfig(config Config) error {
	configPath, _ := getConfigPath()
	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(config)
}
