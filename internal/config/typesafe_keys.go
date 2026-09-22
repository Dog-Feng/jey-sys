package config

import (
	"os"
	"strconv"
	"strings"
)

// exchangeTypeSafeKeyIndex picks the starting key in TYPESAFE_API_KEYS per DEX.
func exchangeTypeSafeKeyIndex(exchange string) int {
	switch strings.ToLower(exchange) {
	case "vanta":
		return 1
	case "lighter", "mock":
		return 0
	default:
		return 0
	}
}

func loadTypeSafeKeys(exchange string) ([]string, int) {
	var keys []string
	if raw := strings.TrimSpace(os.Getenv("TYPESAFE_API_KEYS")); raw != "" {
		keys = append(keys, splitKeyList(raw)...)
	}
	if single := strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY")); single != "" {
		keys = appendUniqueKey(keys, single)
	}
	keys = dedupeKeys(keys)

	start := exchangeTypeSafeKeyIndex(exchange)
	if v := strings.TrimSpace(os.Getenv("TYPESAFE_KEY_INDEX")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			start = n
		}
	}
	if len(keys) > 0 && start >= len(keys) {
		start = start % len(keys)
	}
	return keys, start
}

func splitKeyList(raw string) []string {
	raw = strings.ReplaceAll(raw, "\n", ",")
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func appendUniqueKey(keys []string, k string) []string {
	for _, existing := range keys {
		if existing == k {
			return keys
		}
	}
	return append(keys, k)
}

func dedupeKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}
