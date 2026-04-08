package git

import "testing"

func TestParseRelativeAge(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"3 days ago", "72h0m0s"},
		{"2 weeks ago", "336h0m0s"},
		{"15 minutes ago", "15m0s"},
		{"bad input", "0s"},
	}

	for _, tt := range tests {
		if got := ParseRelativeAge(tt.in).String(); got != tt.want {
			t.Fatalf("ParseRelativeAge(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
