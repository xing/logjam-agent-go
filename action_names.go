package logjam

import (
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

// LegacyActionNameExtractor is an extractor used in older versions of this package. Use
// it if you want to keep old action names in Logjam.
func LegacyActionNameExtractor(r *http.Request) string {
	return legacyActionNameFrom(r.Method, r.URL.EscapedPath())
}

func ignoreActionName(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsDigit(r)
}

func legacyActionNameFrom(method, path string) string {
	parts := legacyActionNameParts(method, path)
	class := strings.Replace(strings.Join(parts[0:len(parts)-1], "::"), "-", "", -1)
	suffix := strings.Replace(strings.ToLower(parts[len(parts)-1]), "-", "_", -1)
	if class == "" {
		class = "Unknown"
	}
	return class + "#" + suffix
}

func legacyActionNameParts(method, path string) []string {
	splitPath := strings.Split(path, "/")
	parts := []string{}
	for _, part := range splitPath {
		if part == "" {
			continue
		}
		if ignoreActionName(part) {
			parts = append(parts, "by_id")
		} else {
			parts = append(parts, strings.Title(part))
			if part == "v1" {
				parts = append(parts, method)
			}
		}
	}
	return parts
}

// DefaultActionNameExtractor replaces slashes with "::" and camel cases the individual
// path segments.
func DefaultActionNameExtractor(r *http.Request) string {
	return defaultActionNameFrom(r.Method, r.URL.EscapedPath())
}

func defaultActionNameFrom(method, path string) string {
	parts := defaultActionNameParts(method, path)
	class := strings.Join(parts, "::")
	return class + "#" + strings.ToLower(method)
}

func defaultActionNameParts(method, path string) []string {
	splitPath := strings.Split(path, "/")
	parts := []string{}
	for _, part := range splitPath {
		if part == "" {
			continue
		}
		if ignoreActionName(part) {
			parts = append(parts, "Id")
		} else {
			parts = append(parts, formatSegment(part))
		}
	}
	return parts
}

func formatSegment(s string) string {
	s = strings.Replace(s, "_", "-", -1)
	parts := strings.Split(s, "-")
	for i, s := range parts {
		parts[i] = strings.Title(s)
	}
	return strings.Join(parts, "")
}
