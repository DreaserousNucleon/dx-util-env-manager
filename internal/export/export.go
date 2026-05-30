package export

import (
	"encoding/base64"
	"fmt"
	"io"
	"sort"
	"strings"
)

// sortedKeys returns map keys in sorted order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// K8sSecret writes a Kubernetes Secret manifest to w.
func K8sSecret(w io.Writer, vars map[string]string, name, namespace string) {
	fmt.Fprintln(w, "apiVersion: v1")
	fmt.Fprintln(w, "kind: Secret")
	fmt.Fprintln(w, "metadata:")
	fmt.Fprintf(w, "  name: %s\n", name)
	if namespace != "" {
		fmt.Fprintf(w, "  namespace: %s\n", namespace)
	}
	fmt.Fprintln(w, "type: Opaque")
	fmt.Fprintln(w, "data:")
	for _, k := range sortedKeys(vars) {
		fmt.Fprintf(w, "  %s: %s\n", k, base64.StdEncoding.EncodeToString([]byte(vars[k])))
	}
}

// K8sConfigMap writes a Kubernetes ConfigMap manifest to w.
func K8sConfigMap(w io.Writer, vars map[string]string, name, namespace string) {
	fmt.Fprintln(w, "apiVersion: v1")
	fmt.Fprintln(w, "kind: ConfigMap")
	fmt.Fprintln(w, "metadata:")
	fmt.Fprintf(w, "  name: %s\n", name)
	if namespace != "" {
		fmt.Fprintf(w, "  namespace: %s\n", namespace)
	}
	fmt.Fprintln(w, "data:")
	for _, k := range sortedKeys(vars) {
		fmt.Fprintf(w, "  %s: %q\n", k, vars[k])
	}
}

// Compose writes a docker-compose environment block to w.
func Compose(w io.Writer, vars map[string]string) {
	fmt.Fprintln(w, "environment:")
	for _, k := range sortedKeys(vars) {
		fmt.Fprintf(w, "  - %s=%s\n", k, vars[k])
	}
}

// DockerEnv writes --env flags for docker run to w.
func DockerEnv(w io.Writer, vars map[string]string) {
	for _, k := range sortedKeys(vars) {
		fmt.Fprintf(w, "--env %s=%s\n", k, escapeShell(vars[k]))
	}
}

func escapeShell(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n\"'\\$`!#&|;(){}") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
