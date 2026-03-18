package ton

import (
	"strings"
)

// IsTonURL reports whether the given string is a .ton URL or bare domain.
// Accepts: "site.ton", "http://site.ton", "https://site.ton", "http://sub.site.ton/path"
func IsTonURL(raw string) bool {
	domain := extractDomain(raw)
	return strings.HasSuffix(domain, ".ton")
}

// NormalizeURL ensures a .ton URL has an http:// prefix.
// TON proxy serves over HTTP (not HTTPS).
// If the input already has http:// it is returned unchanged.
// https:// is replaced with http://.
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "https://") {
		raw = "http://" + raw[len("https://"):]
	}
	if !strings.HasPrefix(raw, "http://") {
		raw = "http://" + raw
	}
	return raw
}

// extractDomain strips scheme and path from a URL to return just the host.
func extractDomain(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "://"); idx != -1 {
		raw = raw[idx+3:]
	}
	if idx := strings.Index(raw, "/"); idx != -1 {
		raw = raw[:idx]
	}
	if idx := strings.Index(raw, "?"); idx != -1 {
		raw = raw[:idx]
	}
	return raw
}
