package ton

import "testing"

func TestIsTonURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"site.ton", true},
		{"http://site.ton", true},
		{"https://site.ton", true},
		{"http://sub.site.ton/path", true},
		{"http://site.ton/page?q=1", true},
		{"site.com", false},
		{"", false},
		{"ton", false},
		{"http://example.com", false},
		{"notaton.url.com", false},
		{"  site.ton  ", true},
	}
	for _, tc := range tests {
		got := IsTonURL(tc.input)
		if got != tc.want {
			t.Errorf("IsTonURL(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"site.ton", "http://site.ton"},
		{"http://site.ton", "http://site.ton"},
		{"https://site.ton", "http://site.ton"},
		{"  site.ton  ", "http://site.ton"},
		{"http://sub.site.ton/path", "http://sub.site.ton/path"},
		{"https://sub.site.ton/path?q=1", "http://sub.site.ton/path?q=1"},
		{"bare.ton", "http://bare.ton"},
	}
	for _, tc := range tests {
		got := NormalizeURL(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
