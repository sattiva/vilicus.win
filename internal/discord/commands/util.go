package commands

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)


var snowflakeRe = regexp.MustCompile(`^\d{15,21}$`)

func ValidSnowflake(s string) bool {
	return snowflakeRe.MatchString(s)
}

func ParseMentionID(arg string) string {
	arg = strings.TrimSpace(arg)
	if strings.HasPrefix(arg, "<@") && strings.HasSuffix(arg, ">") {
		inner := strings.TrimSuffix(strings.TrimPrefix(arg, "<@"), ">")
		inner = strings.TrimPrefix(inner, "!")
		if snowflakeRe.MatchString(inner) {
			return inner
		}
		return ""
	}
	if snowflakeRe.MatchString(arg) {
		return arg
	}
	return ""
}

func ParseIDArg(arg string) string {
	arg = strings.TrimSpace(arg)
	arg = strings.TrimPrefix(arg, "<")
	arg = strings.TrimSuffix(arg, ">")
	arg = strings.TrimLeft(arg, "#@&!")
	if snowflakeRe.MatchString(arg) {
		return arg
	}
	return ""
}

var durationChunkRe = regexp.MustCompile(`(\d+)\s*(d|h|m|s)`)

func ParseDurationArg(raw string) time.Duration {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return 0
	}
	var total time.Duration
	matches := durationChunkRe.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
		return 0
	}
	consumed := 0
	for _, m := range matches {
		consumed += len(m[0])
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || n <= 0 {
			return 0
		}
		switch m[2] {
		case "d":
			total += time.Duration(n) * 24 * time.Hour
		case "h":
			total += time.Duration(n) * time.Hour
		case "m":
			total += time.Duration(n) * time.Minute
		case "s":
			total += time.Duration(n) * time.Second
		}
	}
	if consumed != len(strings.ReplaceAll(raw, " ", "")) {
		return 0
	}
	const maxDur = 365 * 24 * time.Hour
	if total > maxDur {
		return 0
	}
	return total
}

func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	days := int64(d / (24 * time.Hour))
	hours := int64(d/time.Hour) % 24
	mins := int64(d/time.Minute) % 60
	secs := int64(d/time.Second) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd%dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh%dm", hours, mins)
	case mins > 0:
		return fmt.Sprintf("%dm%ds", mins, secs)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

