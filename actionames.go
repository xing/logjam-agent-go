package logjam

import (
	"net/http"
	"strings"
)

// LegacyActionNameExtractor is an extractor used in older versions of this package. Use
// it if you want to keep old action names in Logjam.
func LegacyActionNameExtractor(r *http.Request) string {
	return actionNameFrom(r.Method, r.URL.EscapedPath())
}

var ignoreActionNamePrefixes = []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}

func ignoreActionName(s string) bool {
	for _, prefix := range ignoreActionNamePrefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func actionNameFrom(method, path string) string {
	parts := actionNameParts(method, path)
	class := strings.Replace(strings.Join(parts[0:len(parts)-1], "::"), "-", "", -1)
	suffix := strings.Replace(strings.ToLower(parts[len(parts)-1]), "-", "_", -1)
	return class + "#" + suffix
}

func actionNameParts(method, path string) []string {
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
