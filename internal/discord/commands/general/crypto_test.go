package general

import (
	"strings"
	"testing"
)


func TestNewUUIDv7Layout(t *testing.T) {
	u, err := newUUID(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(u) != 36 || strings.Count(u, "-") != 4 {
		t.Fatalf("malformed uuid: %q", u)
	}
	if u[14] != '7' {
		t.Fatalf("version nibble = %q, want '7'", u[14])
	}
	variant := u[19]
	if variant != '8' && variant != '9' && variant != 'a' && variant != 'b' {
		t.Fatalf("variant char = %q, want 8/9/a/b", variant)
	}
}

func TestNewUUIDv4Layout(t *testing.T) {
	u, err := newUUID(4)
	if err != nil {
		t.Fatal(err)
	}
	if u[14] != '4' {
		t.Fatalf("version nibble = %q, want '4'", u[14])
	}
}

func TestRandStringAlphabetAndBias(t *testing.T) {
	seen := make(map[byte]bool)
	for i := 0; i < 500; i++ {
		s, err := randString(24, tokenCharset)
		if err != nil || len(s) != 24 {
			t.Fatalf("randString: %q err=%v", s, err)
		}
		for j := 0; j < len(s); j++ {
			if !strings.ContainsRune(tokenCharset, rune(s[j])) {
				t.Fatalf("char %q outside alphabet", s[j])
			}
			seen[s[j]] = true
		}
	}
	if len(seen) < 40 {
		t.Fatalf("suspiciously narrow output: only %d distinct chars", len(seen))
	}
}

func TestShannonEntropy(t *testing.T) {
	total, ceiling := shannonEntropy("")
	if total != 0 || ceiling != 0 {
		t.Fatalf("empty string should be zero: %f/%f", total, ceiling)
	}

	total, _ = shannonEntropy("aaaaaaaa")
	if total != 0 {
		t.Fatalf("uniform string entropy = %f, want 0", total)
	}

	total, _ = shannonEntropy("abababab")
	if total != 8 {
		t.Fatalf("two-symbol entropy = %f, want 8", total)
	}

	_, lowCeil := shannonEntropy("aaaa")
	_, hiCeil := shannonEntropy("aA1!")
	if hiCeil <= lowCeil {
		t.Fatalf("charset ceiling did not grow: %f vs %f", hiCeil, lowCeil)
	}
}

func TestParseEmojiTag(t *testing.T) {
	cases := []struct {
		in       string
		id, name string
		anim     bool
	}{
		{"<:pog:123456789012345678>", "123456789012345678", "pog", false},
		{"<a:spin:123456789012345678>", "123456789012345678", "spin", true},
		{":pog:123456789012345678", "123456789012345678", "pog", false},
	}
	for _, c := range cases {
		p := parseEmojiTag(c.in)
		if p == nil || p.ID != c.id || p.Name != c.name || p.Animated != c.anim {
			t.Errorf("parse(%q) = %+v, want id=%s name=%s anim=%v", c.in, p, c.id, c.name, c.anim)
		}
	}
	if parseEmojiTag("<:bad:1>") != nil {
		t.Error("short ids must not parse")
	}
	if parseEmojiTag(":no-underscore!:1") != nil {
		t.Error("invalid names must not parse")
	}
}

func TestChunkLines(t *testing.T) {
	long := strings.Repeat("word ", 3000)
	blocks := chunkLines(long, 100)
	if len(blocks) < 2 {
		t.Fatal("long input should split into multiple blocks")
	}
	for _, b := range blocks {
		if len(b) > 200 {
			t.Fatalf("block too large: %d", len(b))
		}
	}
	multi := "a\nb\nc\n"
	if got := chunkLines(multi, 100); len(got) != 1 || got[0] != multi {
		t.Fatalf("short multi-line passthrough wrong: %q", got)
	}
}

