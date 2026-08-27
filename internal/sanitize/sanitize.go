package sanitize

import "strings"

const zwsp = "\u200b"

func UserText(s string) string {
	if !strings.Contains(s, "@") {
		return s
	}
	s = strings.ReplaceAll(s, "@everyone", "@"+zwsp+"everyone")
	s = strings.ReplaceAll(s, "@here", "@"+zwsp+"here")
	return s
}
