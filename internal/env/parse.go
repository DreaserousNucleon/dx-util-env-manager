package env

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// ValidateKey rejects empty keys and keys containing '='.
func ValidateKey(key string) error {
	if key == "" {
		return fmt.Errorf("key must not be empty")
	}
	if strings.Contains(key, "=") {
		return fmt.Errorf("key must not contain '='")
	}
	if !utf8.ValidString(key) {
		return fmt.Errorf("key must be valid UTF-8")
	}
	return nil
}

// ValidateValue rejects non-UTF-8 values.
func ValidateValue(value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("value must be valid UTF-8")
	}
	return nil
}

// ParseFile reads a .env file supporting:
// - Single and double-quoted values
// - Escape sequences in double-quoted values: \", \\, \n, \r, \t
// - Inline comments (unquoted # after value)
// - Multiline values within quotes
// - export prefix stripping
func ParseFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(string(data))
}

func Parse(input string) (map[string]string, error) {
	result := map[string]string{}
	lines := strings.Split(input, "\n")

	for i := 0; i < len(lines); {
		line := strings.TrimRight(lines[i], "\r")
		trimmed := strings.TrimSpace(line)

		// Skip blank lines and comments.
		if trimmed == "" || trimmed[0] == '#' {
			i++
			continue
		}

		// Strip export prefix.
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimSpace(trimmed[7:])
		}

		key, rest, ok := strings.Cut(trimmed, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: missing '='", i+1)
		}

		key = strings.TrimSpace(key)
		if err := ValidateKey(key); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}

		value, consumed, err := parseValue(rest, lines, i)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}

		if err := ValidateValue(value); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}

		result[key] = value
		i += consumed
	}

	return result, nil
}

// parseValue parses the value portion after '='. Returns the parsed value
// and the number of lines consumed (at least 1).
func parseValue(rest string, lines []string, startLine int) (string, int, error) {
	rest = strings.TrimLeft(rest, " \t")

	if rest == "" {
		return "", 1, nil
	}

	quote := rest[0]

	// Double-quoted: supports escapes and multiline.
	if quote == '"' {
		return parseQuoted(rest[1:], lines, startLine, '"', true)
	}

	// Single-quoted: literal, no escapes, multiline.
	if quote == '\'' {
		return parseQuoted(rest[1:], lines, startLine, '\'', false)
	}

	// Unquoted: strip inline comment.
	value := stripInlineComment(rest)
	return strings.TrimRight(value, " \t"), 1, nil
}

func parseQuoted(after string, lines []string, startLine int, quote byte, escapes bool) (string, int, error) {
	var b strings.Builder
	current := after
	lineCount := 0

	for {
		for j := 0; j < len(current); j++ {
			ch := current[j]

			if escapes && ch == '\\' && j+1 < len(current) {
				next := current[j+1]
				switch next {
				case '"':
					b.WriteByte('"')
				case '\\':
					b.WriteByte('\\')
				case 'n':
					b.WriteByte('\n')
				case 'r':
					b.WriteByte('\r')
				case 't':
					b.WriteByte('\t')
				default:
					b.WriteByte('\\')
					b.WriteByte(next)
				}
				j++
				continue
			}

			if ch == quote {
				return b.String(), lineCount + 1, nil
			}

			b.WriteByte(ch)
		}

		// Value continues on next line.
		lineCount++
		nextIdx := startLine + lineCount
		if nextIdx >= len(lines) {
			return "", 0, fmt.Errorf("unterminated %c quote", quote)
		}
		b.WriteByte('\n')
		current = strings.TrimRight(lines[nextIdx], "\r")
	}
}

func stripInlineComment(s string) string {
	// Find first unescaped # preceded by whitespace.
	for i := 1; i < len(s); i++ {
		if s[i] == '#' && (s[i-1] == ' ' || s[i-1] == '\t') {
			return s[:i]
		}
	}
	return s
}
