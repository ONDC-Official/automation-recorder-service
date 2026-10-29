package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
)

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func mergeMaps(a, b map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func getString(m map[string]any, k string) string {
	v, ok := m[k]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func getInt64(m map[string]any, k string) int64 {
	v, ok := m[k]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

// durationMsOf reads additionalData's duration_ms (FD: the exchange's answer time). It is
// present only when it is a finite, non-negative number of milliseconds, given as a JSON
// number or a numeric string; anything else (absent, null, negative, not a number) is absent,
// so a bad value is never stored as a timing.
func durationMsOf(m map[string]any) (int64, bool) {
	v, ok := m["duration_ms"]
	if !ok || v == nil {
		return 0, false
	}
	var f float64
	switch n := v.(type) {
	case float64:
		f = n
	case float32:
		f = float64(n)
	case int:
		f = float64(n)
	case int64:
		f = float64(n)
	case json.Number:
		parsed, err := n.Float64()
		if err != nil {
			return 0, false
		}
		f = parsed
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0, false
		}
		f = parsed
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0, false
	}
	return int64(math.Round(f)), true
}

func getBool(m map[string]any, k string) bool {
	v, ok := m[k]
	if !ok || v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

func parseEnvSet(v string) map[string]bool {
	res := map[string]bool{}
	for _, p := range strings.Split(v, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		res[p] = true
	}
	return res
}

func envBool(name string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

// Optional helper for parsing ints from env.
func envInt(name string, def int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func uuidV4() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	buf := make([]byte, 36)
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])
	return string(buf), nil
}
