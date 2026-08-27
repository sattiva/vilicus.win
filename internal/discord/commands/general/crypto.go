package general

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"vilicus/internal/components"
	"vilicus/internal/discord/commands"
)



var (
	uuidVerMin = float64(4)
	genLenMin  = float64(4)
)

type UUIDCmd struct{}

func (c *UUIDCmd) Name() string { return "uuid" }
func (c *UUIDCmd) Description() string {
	return "Generate UUIDs (RFC 9562 v7 with timestamp or v4 random)"
}
func (c *UUIDCmd) Category() string  { return "Utility" }
func (c *UUIDCmd) Aliases() []string { return nil }
func (c *UUIDCmd) FastPath() bool    { return true }

func (c *UUIDCmd) RequiredPermissions() *int64 { return nil }

func (c *UUIDCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionInteger,
			Name:        "version",
			Description: "UUID version (default 4)",
			Required:    false,
			MinValue:    &uuidVerMin,
			MaxValue:    7,
		},
	}
}

func (c *UUIDCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	version := 4
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "version" {
			version = int(o.IntValue())
		}
	}
	return c.run(ctx, b, version)
}

func (c *UUIDCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	version := 4
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil || (n != 4 && n != 7) {
			return b.Container(components.TextDisplay{Content: "Version must be 4 or 7."}), nil
		}
		version = n
	}
	return c.run(ctx, b, version)
}

func (c *UUIDCmd) run(_ context.Context, b commands.BotInterface, version int) (*components.Container, error) {
	const count = 5
	var sb strings.Builder
	for n := 0; n < count; n++ {
		u, err := newUUID(version)
		if err != nil {
			return b.Container(components.TextDisplay{Content: "Generation failed: " + err.Error()}), nil
		}
		sb.WriteString("`" + u + "`\n")
	}
	return b.Container(
		components.TextDisplay{Content: fmt.Sprintf("UUID v%d x%d", version, count)},
		components.Separator{Divider: true, Spacing: 1},
		sectionLines(strings.TrimSuffix(sb.String(), "\n")),
	), nil
}

func newUUID(version int) (string, error) {
	var u [16]byte
	if _, err := rand.Read(u[:]); err != nil {
		return "", err
	}
	switch version {
	case 7:
		ms := uint64(time.Now().UnixMilli())
		u[0] = byte(ms >> 40)
		u[1] = byte(ms >> 32)
		u[2] = byte(ms >> 24)
		u[3] = byte(ms >> 16)
		u[4] = byte(ms >> 8)
		u[5] = byte(ms)
		u[6] = (u[6] & 0x0f) | 0x70
	case 4:
		u[6] = (u[6] & 0x0f) | 0x40
	default:
		return "", fmt.Errorf("unsupported version %d", version)
	}
	u[8] = (u[8] & 0x3f) | 0x80

	h := hex.EncodeToString(u[:])
	return strings.Join([]string{h[0:8], h[8:12], h[12:16], h[16:20], h[20:32]}, "-"), nil
}


type HashCmd struct{}

var hashAlgos = map[string]func([]byte) string{
	"sha256": func(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) },
	"sha512": func(b []byte) string { sum := sha512.Sum512(b); return hex.EncodeToString(sum[:]) },
}

func (c *HashCmd) Name() string        { return "hash" }
func (c *HashCmd) Description() string { return "SHA-256/SHA-512 hash of text (hex output)" }
func (c *HashCmd) Category() string    { return "Utility" }
func (c *HashCmd) Aliases() []string   { return nil }
func (c *HashCmd) FastPath() bool      { return true }

func (c *HashCmd) RequiredPermissions() *int64 { return nil }

func (c *HashCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{Name: "algorithm", Description: "sha256 or sha512", Type: discordgo.ApplicationCommandOptionString, Required: false,
			Choices: []*discordgo.ApplicationCommandOptionChoice{
				{Name: "sha256", Value: "sha256"},
				{Name: "sha512", Value: "sha512"},
			}},
		{Name: "text", Description: "Text to hash", Type: discordgo.ApplicationCommandOptionString, Required: true},
	}
}

func (c *HashCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	algo, text := "sha256", ""
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "algorithm":
			algo = o.StringValue()
		case "text":
			text = o.StringValue()
		}
	}
	return c.run(ctx, b, algo, text)
}

func (c *HashCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if len(args) < 2 {
		return b.Container(components.TextDisplay{Content: "Usage: .hash <sha256|sha512> <text>"}), nil
	}
	return c.run(ctx, b, strings.ToLower(args[0]), strings.Join(args[1:], " "))
}

func (c *HashCmd) run(_ context.Context, b commands.BotInterface, algo, text string) (*components.Container, error) {
	fn, ok := hashAlgos[algo]
	if !ok {
		return b.Container(components.TextDisplay{Content: "Unsupported algorithm. md5/sha1 are legacy  -  use sha256 or sha512."}), nil
	}
	return b.Container(
		components.TextDisplay{Content: algo},
		components.Separator{Divider: true, Spacing: 1},
		sectionLines("`"+fn([]byte(text))+"`"),
	), nil
}


type CodecCmd struct {
	Decode bool
}

func (c *CodecCmd) Name() string {
	if c.Decode {
		return "decode"
	}
	return "encode"
}

func (c *CodecCmd) Description() string {
	if c.Decode {
		return "Decode base64, base64url, base32, or hex text"
	}
	return "Encode text as base64, base64url, base32, or hex"
}

func (c *CodecCmd) Category() string  { return "Utility" }
func (c *CodecCmd) Aliases() []string { return nil }
func (c *CodecCmd) FastPath() bool    { return true }

func (c *CodecCmd) RequiredPermissions() *int64 { return nil }

var codecSchemes = map[string]*codecScheme{}

type codecScheme struct {
	encode func([]byte) string
	decode func(string) ([]byte, error)
}

func init() {
	codecSchemes["base64"] = &codecScheme{
		encode: func(b []byte) string { return base64.StdEncoding.EncodeToString(b) },
		decode: func(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) },
	}
	codecSchemes["base64url"] = &codecScheme{
		encode: func(b []byte) string { return base64.URLEncoding.EncodeToString(b) },
		decode: func(s string) ([]byte, error) { return base64.URLEncoding.DecodeString(s) },
	}
	codecSchemes["base32"] = &codecScheme{
		encode: func(b []byte) string { return base32.StdEncoding.EncodeToString(b) },
		decode: func(s string) ([]byte, error) { return base32.StdEncoding.DecodeString(s) },
	}
	codecSchemes["hex"] = &codecScheme{
		encode: func(b []byte) string { return hex.EncodeToString(b) },
		decode: func(s string) ([]byte, error) { return hex.DecodeString(s) },
	}
}

func (c *CodecCmd) Options() []*discordgo.ApplicationCommandOption {
	scheme := &discordgo.ApplicationCommandOption{
		Type: discordgo.ApplicationCommandOptionString, Name: "scheme",
		Description: "Encoding scheme", Required: true,
		Choices: make([]*discordgo.ApplicationCommandOptionChoice, 0, len(codecSchemes)),
	}
	for name := range codecSchemes {
		scheme.Choices = append(scheme.Choices, &discordgo.ApplicationCommandOptionChoice{Name: name, Value: name})
	}
	return []*discordgo.ApplicationCommandOption{
		scheme,
		{Name: "text", Description: "Input text", Type: discordgo.ApplicationCommandOptionString, Required: true},
	}
}

func (c *CodecCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	scheme, text := "", ""
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "scheme":
			scheme = o.StringValue()
		case "text":
			text = o.StringValue()
		}
	}
	return c.run(ctx, b, scheme, text)
}

func (c *CodecCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if len(args) < 2 {
		return b.Container(components.TextDisplay{Content: "Usage: ." + c.Name() + " <base64|base64url|base32|hex> <text>"}), nil
	}
	return c.run(ctx, b, strings.ToLower(args[0]), strings.Join(args[1:], " "))
}

func (c *CodecCmd) run(_ context.Context, b commands.BotInterface, scheme, text string) (*components.Container, error) {
	sc, ok := codecSchemes[scheme]
	if !ok {
		return b.Container(components.TextDisplay{Content: "Unknown scheme. Choose base64, base64url, base32, or hex."}), nil
	}

	var out string
	if c.Decode {
		raw, err := sc.decode(strings.TrimSpace(text))
		if err != nil {
			return b.Container(components.TextDisplay{Content: "Not valid " + scheme + ": " + err.Error()}), nil
		}
		out = string(raw)
	} else {
		out = sc.encode([]byte(text))
	}
	if out == "" {
		out = "(empty result)"
	}
	if len(out) > 1800 {
		out = out[:1800] + "...(truncated)"
	}
	return b.Container(
		components.TextDisplay{Content: scheme + " -> " + map[bool]string{true: "decoded", false: "encoded"}[c.Decode]},
		components.Separator{Divider: true, Spacing: 1},
		sectionLines("```\n"+out+"\n```"),
	), nil
}


type GenCmd struct{}

const (
	genDefaultLen = 32
	genMaxLen     = 1024
)

var (
	passwordCharset = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%^&*()-_=+[]{}?"
	tokenCharset    = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

func (c *GenCmd) Name() string { return "gen" }
func (c *GenCmd) Description() string {
	return "Generate a password, token, hex string, or base64 (crypto/rand)"
}
func (c *GenCmd) Category() string  { return "Utility" }
func (c *GenCmd) Aliases() []string { return []string{"generate"} }
func (c *GenCmd) FastPath() bool    { return true }

func (c *GenCmd) RequiredPermissions() *int64 { return nil }

func (c *GenCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{Name: "kind", Description: "What to generate", Type: discordgo.ApplicationCommandOptionString, Required: false,
			Choices: []*discordgo.ApplicationCommandOptionChoice{
				{Name: "password", Value: "password"},
				{Name: "token", Value: "token"},
				{Name: "hex", Value: "hex"},
				{Name: "base64", Value: "base64"},
			}},
		{Name: "length", Description: "Length (default 32, max 1024)", Type: discordgo.ApplicationCommandOptionInteger, Required: false,
			MinValue: &genLenMin, MaxValue: genMaxLen},
	}
}

func (c *GenCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	kind, length := "password", genDefaultLen
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "kind":
			kind = o.StringValue()
		case "length":
			length = int(o.IntValue())
		}
	}
	return c.run(ctx, b, kind, length)
}

func (c *GenCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	kind, length := "password", genDefaultLen
	if len(args) > 0 {
		kind = strings.ToLower(args[0])
	}
	if len(args) > 1 {
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 4 || n > genMaxLen {
			return b.Container(components.TextDisplay{Content: fmt.Sprintf("Length must be 4-%d.", genMaxLen)}), nil
		}
		length = n
	}
	return c.run(ctx, b, kind, length)
}

func (c *GenCmd) run(_ context.Context, b commands.BotInterface, kind string, length int) (*components.Container, error) {
	if length <= 0 {
		length = genDefaultLen
	}
	if length > genMaxLen {
		length = genMaxLen
	}

	var out, note string
	switch kind {
	case "password":
		s, err := randString(length, passwordCharset)
		if err != nil {
			return b.Container(components.TextDisplay{Content: "Generation failed: " + err.Error()}), nil
		}
		out, note = s, strconv.Itoa(len(passwordCharset))+"-char alphabet"
	case "token":
		s, err := randString(length, tokenCharset)
		if err != nil {
			return b.Container(components.TextDisplay{Content: "Generation failed: " + err.Error()}), nil
		}
		out, note = s, "alphanumeric"
	case "hex":
		buf := make([]byte, (length+1)/2)
		if _, err := rand.Read(buf); err != nil {
			return b.Container(components.TextDisplay{Content: "Generation failed: " + err.Error()}), nil
		}
		out, note = hex.EncodeToString(buf)[:length], "random bytes"
	case "base64":
		buf := make([]byte, length)
		if _, err := rand.Read(buf); err != nil {
			return b.Container(components.TextDisplay{Content: "Generation failed: " + err.Error()}), nil
		}
		out, note = base64.RawURLEncoding.EncodeToString(buf)[:length], "url-safe raw encoding"
	default:
		return b.Container(components.TextDisplay{Content: "Kind must be password, token, hex, or base64."}), nil
	}

	return b.Container(
		components.TextDisplay{Content: "Generated (" + kind + ", " + itoa64(int64(length)) + " chars, " + note + ")"},
		components.Separator{Divider: true, Spacing: 1},
		sectionLines("`"+out+"`"),
	), nil
}

func randString(n int, charset string) (string, error) {
	max := byte(256 / len(charset) * len(charset))
	out := make([]byte, 0, n)
	buf := make([]byte, n*2)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, r := range buf {
			if r < max {
				out = append(out, charset[r%byte(len(charset))])
				if len(out) == n {
					break
				}
			}
		}
	}
	return string(out), nil
}


type EntropyCmd struct{}

func (c *EntropyCmd) Name() string        { return "entropy" }
func (c *EntropyCmd) Description() string { return "Shannon entropy estimate for a password or string" }
func (c *EntropyCmd) Category() string    { return "Utility" }
func (c *EntropyCmd) Aliases() []string   { return nil }
func (c *EntropyCmd) FastPath() bool      { return true }

func (c *EntropyCmd) RequiredPermissions() *int64 { return nil }

func (c *EntropyCmd) Options() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{Name: "text", Description: "String to measure", Type: discordgo.ApplicationCommandOptionString, Required: true},
	}
}

func (c *EntropyCmd) ExecuteSlash(ctx context.Context, b commands.BotInterface, s *discordgo.Session, i *discordgo.InteractionCreate) (*components.Container, error) {
	text := ""
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "text" {
			text = o.StringValue()
		}
	}
	return c.run(ctx, b, text)
}

func (c *EntropyCmd) ExecutePrefix(ctx context.Context, b commands.BotInterface, s *discordgo.Session, m *discordgo.MessageCreate, args []string) (*components.Container, error) {
	if len(args) == 0 {
		return b.Container(components.TextDisplay{Content: "Usage: .entropy <text>"}), nil
	}
	return c.run(ctx, b, strings.Join(args, " "))
}

func (c *EntropyCmd) run(_ context.Context, b commands.BotInterface, text string) (*components.Container, error) {
	shannonBits, poolBits := shannonEntropy(text)

	verdict := "very weak"
	switch {
	case shannonBits >= 128:
		verdict = "excellent"
	case shannonBits >= 60:
		verdict = "strong"
	case shannonBits >= 36:
		verdict = "fair"
	case shannonBits >= 28:
		verdict = "weak"
	}

	return b.Container(
		components.TextDisplay{Content: "Entropy Analysis"},
		components.Separator{Divider: true, Spacing: 1},
		sectionLines(
			fmt.Sprintf("Length: %d characters", len(text)),
			fmt.Sprintf("Shannon entropy: %.1f bits (%s)", shannonBits, verdict),
			fmt.Sprintf("Charset ceiling: %.1f bits", poolBits),
		),
	), nil
}

func shannonEntropy(s string) (total float64, ceiling float64) {
	if len(s) == 0 {
		return 0, 0
	}
	freq := make(map[rune]int)
	pool := 0.0
	lower, upper, digit, symbol := false, false, false, false
	for _, r := range s {
		freq[r]++
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		default:
			symbol = true
		}
	}
	if lower {
		pool += 26
	}
	if upper {
		pool += 26
	}
	if digit {
		pool += 10
	}
	if symbol {
		pool += 33
	}

	n := float64(len([]rune(s)))
	for _, f := range freq {
		p := float64(f) / n
		total -= p * math.Log2(p)
	}
	total *= n
	ceiling = n * math.Log2(math.Max(pool, 1))
	return total, ceiling
}

