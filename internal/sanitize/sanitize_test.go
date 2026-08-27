package sanitize

import "testing"

func TestUserText(t *testing.T) {
	const zwsp = "\u200b"
	for _, tc := range []struct{ in, want string }{
		{"@everyone hello", "@" + zwsp + "everyone hello"},
		{"@here", "@" + zwsp + "here"},
		{"hey @everyone and @here", "hey @" + zwsp + "everyone and @" + zwsp + "here"},
		{"@everyone@here tight", "@" + zwsp + "everyone@" + zwsp + "here tight"},
		{"@Everyone", "@Everyone"},
		{"user@everywhere", "user@everywhere"},
		{"no mention here", "no mention here"},
		{"", ""},
	} {
		if got := UserText(tc.in); got != tc.want {
			t.Errorf("UserText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
