package app

import (
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dx-util-env-manager/internal/config"
	cryptoutil "dx-util-env-manager/internal/crypto"
	"dx-util-env-manager/internal/env"
	exportutil "dx-util-env-manager/internal/export"
	"dx-util-env-manager/internal/paths"
	runner "dx-util-env-manager/internal/runtime"
	"dx-util-env-manager/internal/store"
	"dx-util-env-manager/internal/version"
)

type Context struct {
	Out    io.Writer
	Err    io.Writer
	Store  store.EnvStore
	Config config.Config
	Mode   outputMode
}

func Run(args []string, out io.Writer, errW io.Writer) int {
	if len(args) == 0 {
		args = []string{"de"}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(errW, "config error:", err)
		return 1
	}

	mode := defaultOutputMode()

	// Apply config file preferences.
	if cfg.Reveal {
		mode.Reveal = true
	}
	if cfg.Color == "always" {
		mode.IsTTY = true
	} else if cfg.Color == "never" {
		mode.IsTTY = false
	}

	// Parse global flags before the subcommand.
	args, mode, cfg = parseGlobalFlags(args, mode, cfg)

	ctx := &Context{
		Out:    out,
		Err:    errW,
		Store:  store.NewJSONStore(cfg.StorePath, cfg.Profile, cfg.AllowReinitOnDecryptFail, errW),
		Config: cfg,
		Mode:   mode,
	}

	return run(ctx, args)
}

// parseGlobalFlags extracts --verbose, --json, and --profile from args[1:] before the subcommand.
func parseGlobalFlags(args []string, m outputMode, cfg config.Config) ([]string, outputMode, config.Config) {
	if len(args) < 2 {
		return args, m, cfg
	}

	cleaned := []string{args[0]}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--verbose":
			m.Verbose = true
		case "--json":
			m.JSON = true
		case "--profile":
			if i+1 < len(args) {
				i++
				cfg.Profile = args[i]
			}
		default:
			cleaned = append(cleaned, args[i])
		}
	}
	return cleaned, m, cfg
}

func run(ctx *Context, args []string) int {
	if len(args) < 2 {
		printUsage(ctx.Out)
		return 2
	}

	cmd := args[1]

	// First-run check for commands that need the store.
	if needsStore(cmd) {
		if showFirstRun(ctx) {
			return 0
		}
	}

	switch cmd {
	case "help", "-h", "--help":
		printUsage(ctx.Out)
		return 0
	case "version":
		fmt.Fprintln(ctx.Out, version.String())
		return 0
	case "exec":
		return runExec(ctx, args[2:])
	case "init":
		return runInit(ctx, args[2:])
	case "migrate":
		return runMigrate(ctx, args[2:])
	case "set":
		return runSet(ctx, args[2:])
	case "get":
		return runGet(ctx, args[2:])
	case "unset":
		return runUnset(ctx, args[2:])
	case "list":
		return runList(ctx, args[2:])
	case "key":
		return runKey(ctx, args[2:])
	case "doctor":
		return runDoctor(ctx)
	case "export":
		return runExport(ctx, args[2:])
	case "profile":
		return runProfile(ctx, args[2:])
	case "config":
		return runConfig(ctx, args[2:])
	case "completion":
		return runCompletion(ctx, args[2:])
	default:
		return runExec(ctx, args[1:])
	}
}

// storeErr wraps a store error with an actionable hint.
func storeErr(errW io.Writer, err error) int {
	if errors.Is(err, cryptoutil.ErrKeyNotFound) {
		fmt.Fprintln(errW, "error: master key not found in OS keychain")
		fmt.Fprintln(errW, "  hint: run 'de init' to create a new store, or 'de key import --file PATH' to restore a backup")
		return 1
	}
	msg := err.Error()
	if strings.Contains(msg, "decrypt store") {
		fmt.Fprintln(errW, "error:", msg)
		fmt.Fprintln(errW, "  hint: the store may have been encrypted with a different key; try 'de key import' to restore the correct key")
		return 1
	}
	fmt.Fprintln(errW, "store error:", err)
	return 1
}

// resolveDir parses the --dir flag and returns the canonical directory.
func resolveDir(fs *flag.FlagSet, args []string, errW io.Writer) (string, []string, int) {
	dirFlag := fs.String("dir", "", "directory context (default: current)")
	if err := fs.Parse(args); err != nil {
		return "", nil, 2
	}
	dir, err := paths.CanonicalDir(*dirFlag)
	if err != nil {
		fmt.Fprintln(errW, "dir error:", err)
		return "", nil, 1
	}
	return dir, fs.Args(), 0
}

func verboseLog(ctx *Context, dir string, varCount int) {
	if !ctx.Mode.Verbose {
		return
	}
	fmt.Fprintf(ctx.Err, "[verbose] dir: %s\n", dir)
	fmt.Fprintf(ctx.Err, "[verbose] store: %s\n", ctx.Config.StorePath)
	fmt.Fprintf(ctx.Err, "[verbose] variables: %d\n", varCount)
}

func runSet(ctx *Context, args []string) int {
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.SetOutput(ctx.Err)
	dir, remaining, code := resolveDir(fs, args, ctx.Err)
	if code != 0 {
		return code
	}

	if len(remaining) == 0 {
		fmt.Fprintln(ctx.Err, "usage: de set KEY=VALUE [--dir PATH]")
		return 2
	}

	kv := remaining[0]
	key, value, ok := strings.Cut(kv, "=")
	if !ok {
		fmt.Fprintln(ctx.Err, "usage: de set KEY=VALUE [--dir PATH]")
		return 2
	}

	if err := env.ValidateKey(key); err != nil {
		fmt.Fprintln(ctx.Err, "invalid key:", err)
		return 2
	}
	if err := env.ValidateValue(value); err != nil {
		fmt.Fprintln(ctx.Err, "invalid value:", err)
		return 2
	}

	if err := ctx.Store.SetVar(dir, key, value); err != nil {
		return storeErr(ctx.Err, err)
	}

	fmt.Fprintf(ctx.Out, "set %s\n", key)
	return 0
}

func runGet(ctx *Context, args []string) int {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	fs.SetOutput(ctx.Err)
	reveal := fs.Bool("reveal", false, "show value in plaintext")
	dirFlag := fs.String("dir", "", "directory context (default: current)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintln(ctx.Err, "usage: de get KEY [--dir PATH] [--reveal]")
		return 2
	}

	dir, err := paths.CanonicalDir(*dirFlag)
	if err != nil {
		fmt.Fprintln(ctx.Err, "dir error:", err)
		return 1
	}

	key := remaining[0]
	if err := env.ValidateKey(key); err != nil {
		fmt.Fprintln(ctx.Err, "invalid key:", err)
		return 2
	}

	value, ok, err := ctx.Store.GetVar(dir, key)
	if err != nil {
		return storeErr(ctx.Err, err)
	}
	if !ok {
		fmt.Fprintf(ctx.Err, "key %q not found\n", key)
		return 1
	}

	m := ctx.Mode
	m.Reveal = m.Reveal || *reveal

	verboseLog(ctx, dir, 1)

	if m.JSON {
		printVarsJSON(ctx.Out, map[string]string{key: value}, m)
	} else {
		printKV(ctx.Out, key, value, m)
	}
	return 0
}

func runUnset(ctx *Context, args []string) int {
	fs := flag.NewFlagSet("unset", flag.ContinueOnError)
	fs.SetOutput(ctx.Err)
	dir, remaining, code := resolveDir(fs, args, ctx.Err)
	if code != 0 {
		return code
	}

	if len(remaining) == 0 {
		fmt.Fprintln(ctx.Err, "usage: de unset KEY [--dir PATH]")
		return 2
	}

	key := remaining[0]
	if err := env.ValidateKey(key); err != nil {
		fmt.Fprintln(ctx.Err, "invalid key:", err)
		return 2
	}

	if err := ctx.Store.UnsetVar(dir, key); err != nil {
		return storeErr(ctx.Err, err)
	}

	fmt.Fprintf(ctx.Out, "unset %s\n", key)
	return 0
}

func runList(ctx *Context, args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(ctx.Err)
	reveal := fs.Bool("reveal", false, "show values in plaintext")
	dirFlag := fs.String("dir", "", "directory context (default: current)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	dir, err := paths.CanonicalDir(*dirFlag)
	if err != nil {
		fmt.Fprintln(ctx.Err, "dir error:", err)
		return 1
	}

	vars, err := ctx.Store.ListVars(dir)
	if err != nil {
		return storeErr(ctx.Err, err)
	}

	verboseLog(ctx, dir, len(vars))

	if len(vars) == 0 {
		if !ctx.Mode.JSON {
			fmt.Fprintln(ctx.Out, "no variables set")
		} else {
			printVarsJSON(ctx.Out, vars, ctx.Mode)
		}
		return 0
	}

	m := ctx.Mode
	m.Reveal = m.Reveal || *reveal
	printVars(ctx.Out, vars, m)
	return 0
}

func runExec(ctx *Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(ctx.Err, "missing command to execute")
		printUsage(ctx.Err)
		return 2
	}

	if args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(ctx.Err, "missing command to execute")
		printUsage(ctx.Err)
		return 2
	}

	dir, err := paths.CanonicalDir("")
	if err != nil {
		fmt.Fprintln(ctx.Err, "dir error:", err)
		return 1
	}

	envMap, err := ctx.Store.EnvForDir(dir)
	if err != nil {
		return storeErr(ctx.Err, err)
	}

	verboseLog(ctx, dir, len(envMap))

	merged := env.Merge(os.Environ(), envMap)
	exitCode, err := runner.Exec(args[0], args[1:], merged, ctx.Out, ctx.Err)
	if err != nil {
		fmt.Fprintln(ctx.Err, "exec error:", err)
		return 1
	}

	return exitCode
}

func runInit(ctx *Context, args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(ctx.Err)
	dirFlag := fs.String("dir", "", "directory to bind (default: current)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	dir, err := paths.CanonicalDir(*dirFlag)
	if err != nil {
		fmt.Fprintln(ctx.Err, "dir error:", err)
		return 1
	}

	if err := ctx.Store.SetEnvForDir(dir, map[string]string{}); err != nil {
		return storeErr(ctx.Err, err)
	}

	fmt.Fprintf(ctx.Out, "initialized %s\n", dir)
	return 0
}

func runMigrate(ctx *Context, args []string) int {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(ctx.Err)
	fileFlag := fs.String("file", ".env", "path to env file")
	dirFlag := fs.String("dir", "", "directory to bind (default: current)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	envMap, err := env.ParseFile(*fileFlag)
	if err != nil {
		fmt.Fprintln(ctx.Err, "parse error:", err)
		return 1
	}
	if len(envMap) == 0 {
		fmt.Fprintln(ctx.Err, "no variables found in file")
		return 1
	}

	dir, err := paths.CanonicalDir(*dirFlag)
	if err != nil {
		fmt.Fprintln(ctx.Err, "dir error:", err)
		return 1
	}

	if err := ctx.Store.SetEnvForDir(dir, envMap); err != nil {
		return storeErr(ctx.Err, err)
	}

	fmt.Fprintf(ctx.Out, "migrated %d variables for %s\n", len(envMap), dir)
	return 0
}

// --- export ---

func runExport(ctx *Context, args []string) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(ctx.Err)
	format := fs.String("format", "dotenv", "output format: dotenv, json, k8s-secret, k8s-configmap, compose, docker-env")
	reveal := fs.Bool("reveal", false, "show values in plaintext")
	dirFlag := fs.String("dir", "", "directory context (default: current)")
	name := fs.String("name", "app-env", "resource name for k8s manifests")
	namespace := fs.String("namespace", "", "namespace for k8s manifests")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	dir, err := paths.CanonicalDir(*dirFlag)
	if err != nil {
		fmt.Fprintln(ctx.Err, "dir error:", err)
		return 1
	}

	vars, err := ctx.Store.ListVars(dir)
	if err != nil {
		return storeErr(ctx.Err, err)
	}

	m := ctx.Mode
	m.Reveal = m.Reveal || *reveal

	verboseLog(ctx, dir, len(vars))

	// Container formats always reveal values (they are the payload).
	revealed := vars
	if !m.Reveal && (*format == "dotenv" || *format == "json") {
		revealed = redactMap(vars)
	}

	switch *format {
	case "dotenv":
		keys := sortedKeys(revealed)
		for _, k := range keys {
			fmt.Fprintf(ctx.Out, "%s=%s\n", k, revealed[k])
		}
	case "json":
		m.JSON = true
		printVarsJSON(ctx.Out, vars, m)
	case "k8s-secret":
		exportutil.K8sSecret(ctx.Out, vars, *name, *namespace)
	case "k8s-configmap":
		exportutil.K8sConfigMap(ctx.Out, vars, *name, *namespace)
	case "compose":
		exportutil.Compose(ctx.Out, vars)
	case "docker-env":
		exportutil.DockerEnv(ctx.Out, vars)
	default:
		fmt.Fprintf(ctx.Err, "unsupported format: %s (use dotenv, json, k8s-secret, k8s-configmap, compose, or docker-env)\n", *format)
		return 2
	}

	return 0
}

func redactMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k := range m {
		out[k] = "****"
	}
	return out
}

// --- key subcommand ---

func runKey(ctx *Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(ctx.Err, "usage: de key <rotate|export|import|purge>")
		return 2
	}

	switch args[0] {
	case "rotate":
		return runKeyRotate(ctx)
	case "export":
		return runKeyExport(ctx, args[1:])
	case "import":
		return runKeyImport(ctx, args[1:])
	case "purge":
		return runKeyPurge(ctx, args[1:])
	default:
		fmt.Fprintf(ctx.Err, "unknown key subcommand: %s\n", args[0])
		return 2
	}
}

func runKeyRotate(ctx *Context) int {
	oldKey, err := cryptoutil.GetKey()
	if err != nil {
		fmt.Fprintln(ctx.Err, "keychain error:", err)
		return 1
	}

	newKey := make([]byte, cryptoutil.KeyLength)
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		fmt.Fprintln(ctx.Err, "generate key:", err)
		return 1
	}

	if err := ctx.Store.Rotate(oldKey, newKey); err != nil {
		fmt.Fprintln(ctx.Err, "rotate error:", err)
		return 1
	}

	fmt.Fprintln(ctx.Out, "key rotated successfully")
	return 0
}

func runKeyExport(ctx *Context, args []string) int {
	fs := flag.NewFlagSet("key export", flag.ContinueOnError)
	fs.SetOutput(ctx.Err)
	fileFlag := fs.String("file", "", "output file path (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *fileFlag == "" {
		fmt.Fprintln(ctx.Err, "usage: de key export --file PATH")
		return 2
	}

	masterKey, err := cryptoutil.GetKey()
	if err != nil {
		fmt.Fprintln(ctx.Err, "keychain error:", err)
		return 1
	}

	fmt.Fprint(ctx.Err, "Enter passphrase: ")
	passphrase, err := readPassphrase()
	if err != nil {
		fmt.Fprintln(ctx.Err, "\npassphrase error:", err)
		return 1
	}
	fmt.Fprintln(ctx.Err)

	if len(passphrase) == 0 {
		fmt.Fprintln(ctx.Err, "passphrase must not be empty")
		return 2
	}

	blob, err := cryptoutil.ExportKey(masterKey, passphrase)
	if err != nil {
		fmt.Fprintln(ctx.Err, "export error:", err)
		return 1
	}

	if err := os.WriteFile(*fileFlag, blob, 0600); err != nil {
		fmt.Fprintln(ctx.Err, "write error:", err)
		return 1
	}

	fmt.Fprintf(ctx.Out, "key exported to %s\n", *fileFlag)
	return 0
}

func runKeyImport(ctx *Context, args []string) int {
	fs := flag.NewFlagSet("key import", flag.ContinueOnError)
	fs.SetOutput(ctx.Err)
	fileFlag := fs.String("file", "", "backup file path (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *fileFlag == "" {
		fmt.Fprintln(ctx.Err, "usage: de key import --file PATH")
		return 2
	}

	blob, err := os.ReadFile(*fileFlag)
	if err != nil {
		fmt.Fprintln(ctx.Err, "read error:", err)
		return 1
	}

	fmt.Fprint(ctx.Err, "Enter passphrase: ")
	passphrase, err := readPassphrase()
	if err != nil {
		fmt.Fprintln(ctx.Err, "\npassphrase error:", err)
		return 1
	}
	fmt.Fprintln(ctx.Err)

	masterKey, err := cryptoutil.ImportKey(blob, passphrase)
	if err != nil {
		fmt.Fprintln(ctx.Err, "import error:", err)
		return 1
	}

	if err := cryptoutil.SetKey(masterKey); err != nil {
		fmt.Fprintln(ctx.Err, "keychain error:", err)
		return 1
	}

	fmt.Fprintln(ctx.Out, "key imported successfully")
	return 0
}

// --- doctor ---

func runDoctor(ctx *Context) int {
	res := ctx.Store.Doctor()

	fmt.Fprintf(ctx.Out, "folders: %d\n", res.FolderCount)
	fmt.Fprintf(ctx.Out, "variables: %d\n", res.TotalVars)

	for _, e := range res.Errors {
		fmt.Fprintf(ctx.Out, "error: %s\n", e)
	}

	if res.OK {
		fmt.Fprintln(ctx.Out, "status: ok")
		return 0
	}
	fmt.Fprintln(ctx.Out, "status: errors detected")
	return 1
}

// --- config ---

func runConfig(ctx *Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(ctx.Err, "usage: de config <path|edit|show>")
		return 2
	}

	switch args[0] {
	case "path":
		fmt.Fprintln(ctx.Out, config.ConfigFilePath(ctx.Config))
		return 0
	case "show":
		path := config.ConfigFilePath(ctx.Config)
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprint(ctx.Out, config.DefaultConfigTemplate)
			return 0
		}
		ctx.Out.Write(data)
		return 0
	case "edit":
		return runConfigEdit(ctx)
	default:
		fmt.Fprintf(ctx.Err, "unknown config subcommand: %s\n", args[0])
		return 2
	}
}

func runConfigEdit(ctx *Context) int {
	path := config.ConfigFilePath(ctx.Config)

	// Ensure file exists with default template.
	if _, err := os.Stat(path); err != nil {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			fmt.Fprintln(ctx.Err, "error:", err)
			return 1
		}
		if err := os.WriteFile(path, []byte(config.DefaultConfigTemplate), 0600); err != nil {
			fmt.Fprintln(ctx.Err, "error:", err)
			return 1
		}
	}

	editor, err := resolveEditor()
	if err != nil {
		fmt.Fprintln(ctx.Err, err)
		return 1
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = ctx.Out
	cmd.Stderr = ctx.Err

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(ctx.Err, "editor error: %v\n", err)
		return 1
	}
	return 0
}

func resolveEditor() (string, error) {
	for _, env := range []string{"DE_EDITOR", "VISUAL", "EDITOR"} {
		if v := os.Getenv(env); v != "" {
			return v, nil
		}
	}
	// Platform fallback.
	if isWindows() {
		return "notepad.exe", nil
	}
	for _, name := range []string{"nano", "vi"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no editor found; set EDITOR or DE_EDITOR environment variable")
}

func isWindows() bool {
	return os.PathSeparator == '\\' && os.PathListSeparator == ';'
}

// --- profile ---

func runProfile(ctx *Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(ctx.Out, ctx.Config.Profile)
		return 0
	}

	switch args[0] {
	case "list":
		for _, p := range config.AllowedProfiles {
			if p == ctx.Config.Profile {
				fmt.Fprintf(ctx.Out, "* %s\n", p)
			} else {
				fmt.Fprintf(ctx.Out, "  %s\n", p)
			}
		}
		return 0
	case "set":
		if len(args) < 2 {
			fmt.Fprintln(ctx.Err, "usage: de profile set <name>")
			return 2
		}
		name := args[1]
		if err := config.ValidateProfile(name); err != nil {
			fmt.Fprintln(ctx.Err, err)
			return 2
		}
		if err := config.SetProfileInFile(ctx.Config, name); err != nil {
			fmt.Fprintln(ctx.Err, "config write error:", err)
			return 1
		}
		fmt.Fprintf(ctx.Out, "profile set to %s\n", name)
		return 0
	default:
		fmt.Fprintf(ctx.Err, "unknown profile subcommand: %s\n", args[0])
		return 2
	}
}

// --- first-run ---

func needsStore(cmd string) bool {
	switch cmd {
	case "help", "-h", "--help", "version", "init", "completion", "profile", "config":
		return false
	case "key":
		return false // key import/export handle their own messaging
	}
	return true
}

func showFirstRun(ctx *Context) bool {
	// Check if store file exists.
	if _, err := os.Stat(ctx.Config.StorePath); err == nil {
		return false
	}
	// Check if keychain has a key.
	if _, err := cryptoutil.GetKey(); err == nil {
		return false
	}

	fmt.Fprintln(ctx.Err, "Welcome to de — the directory-aware env runner.")
	fmt.Fprintln(ctx.Err, "")
	fmt.Fprintln(ctx.Err, "No encrypted store found. To get started:")
	fmt.Fprintln(ctx.Err, "  de init              Create a store and master key for the current directory.")
	fmt.Fprintln(ctx.Err, "  de key import --file  Restore an existing master key from a backup.")
	fmt.Fprintln(ctx.Err, "")
	fmt.Fprintf(ctx.Err, "Store location: %s\n", ctx.Config.StorePath)
	fmt.Fprintf(ctx.Err, "Config file:    %s\n", config.ConfigFilePath(ctx.Config))
	fmt.Fprintln(ctx.Err, "")
	fmt.Fprintln(ctx.Err, "The master key will be stored securely in your OS keychain.")
	return true
}

// --- key purge ---

func runKeyPurge(ctx *Context, args []string) int {
	for _, a := range args {
		if a == "--confirm" {
			if err := cryptoutil.DeleteKey(); err != nil {
				fmt.Fprintln(ctx.Err, "purge error:", err)
				return 1
			}
			fmt.Fprintln(ctx.Out, "master key removed from OS keychain")
			return 0
		}
	}

	fmt.Fprintln(ctx.Err, "usage: de key purge --confirm")
	fmt.Fprintln(ctx.Err, "  This permanently removes the master key from the OS keychain.")
	fmt.Fprintln(ctx.Err, "  Your store will become unreadable unless you have a key backup.")
	return 2
}
