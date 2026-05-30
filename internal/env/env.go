package env

import (
	"runtime"
	"strings"
)

func Merge(base []string, extra map[string]string) []string {
	out := make([]string, len(base))
	copy(out, base)

	index := map[string]int{}
	for i, kv := range out {
		index[normalizeKey(splitKey(kv))] = i
	}

	for k, v := range extra {
		key := normalizeKey(k)
		if i, ok := index[key]; ok {
			out[i] = k + "=" + v
		} else {
			out = append(out, k+"="+v)
		}
	}

	return out
}

func splitKey(kv string) string {
	key, _, _ := strings.Cut(kv, "=")
	return key
}

func normalizeKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}
