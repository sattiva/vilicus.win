package components

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)


type ComponentRef struct {
	NS      string
	Action  string
	Payload []byte
	Expiry  time.Time
	Raw     string
}

func BuildCustomID(ns, action string, payload []byte, expiry time.Time) string {
	exp := int64(0)
	if !expiry.IsZero() {
		exp = expiry.Unix()
	}
	return fmt.Sprintf("%s:%s:%s:%d", ns, action, base64.RawURLEncoding.EncodeToString(payload), exp)
}

func ParseCustomID(raw string) (*ComponentRef, error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 4 {
		return nil, fmt.Errorf("malformed custom id")
	}
	ns, action, payB64, expS := parts[0], parts[1], parts[2], parts[3]
	if len(ns) < 2 || len(ns) > 12 {
		return nil, fmt.Errorf("bad namespace")
	}
	if len(action) < 1 || len(action) > 16 {
		return nil, fmt.Errorf("bad action")
	}
	payload, err := base64.RawURLEncoding.DecodeString(payB64)
	if err != nil {
		return nil, fmt.Errorf("bad payload encoding")
	}
	ref := &ComponentRef{NS: strings.ToLower(ns), Action: action, Payload: payload, Raw: raw}
	if exp, perr := strconv.ParseInt(expS, 10, 64); perr == nil && exp > 0 {
		ref.Expiry = time.Unix(exp, 0)
	}
	return ref, nil
}

