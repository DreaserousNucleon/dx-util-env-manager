package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"golang.org/x/term"
)

// outputMode controls how key-value pairs are rendered.
type outputMode struct {
	Reveal  bool
	JSON    bool
	IsTTY   bool
	Verbose bool
}

func defaultOutputMode() outputMode {
	fd := int(os.Stdout.Fd())
	return outputMode{
		Reveal: os.Getenv("DE_REVEAL") == "1",
		IsTTY:  term.IsTerminal(fd),
	}
}

// printKV prints a single key-value pair respecting redaction, color, and JSON.
func printKV(w io.Writer, key, value string, m outputMode) {
	if m.JSON {
		return // JSON mode uses printVarsJSON instead
	}
	display := "****"
	if m.Reveal {
		display = value
	}
	if m.IsTTY {
		// bold key, reset after
		fmt.Fprintf(w, "\033[1m%s\033[0m=%s\n", key, display)
	} else {
		fmt.Fprintf(w, "%s=%s\n", key, display)
	}
}

// printVars prints a sorted map of variables.
func printVars(w io.Writer, vars map[string]string, m outputMode) {
	if m.JSON {
		printVarsJSON(w, vars, m)
		return
	}

	keys := sortedKeys(vars)
	for _, k := range keys {
		printKV(w, k, vars[k], m)
	}
}

func printVarsJSON(w io.Writer, vars map[string]string, m outputMode) {
	out := make(map[string]string, len(vars))
	for k, v := range vars {
		if m.Reveal {
			out[k] = v
		} else {
			out[k] = "****"
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(out) //nolint:errcheck
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
