package store

import (
	"encoding/json"
	"strings"
)

func EmailSuffix(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	i := strings.LastIndex(email, "@")
	if i < 0 || i == len(email)-1 {
		return ""
	}
	return NormalizeSuffix(email[i+1:])
}

func NormalizeSuffix(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimPrefix(s, "@")
	s = strings.Trim(s, ".")
	return s
}

func NormalizeSuffixList(list []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(list))
	for _, raw := range list {
		s := NormalizeSuffix(raw)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func SuffixBlacklisted(list []string, suffix string) bool {
	suffix = NormalizeSuffix(suffix)
	if suffix == "" {
		return false
	}
	for _, item := range list {
		if item == suffix {
			return true
		}
	}
	return false
}

func EncodeSuffixList(list []string) string {
	list = NormalizeSuffixList(list)
	if len(list) == 0 {
		return "[]"
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func DecodeSuffixList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var list []string
		if err := json.Unmarshal([]byte(raw), &list); err == nil {
			return NormalizeSuffixList(list)
		}
	}
	var list []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == ',' || r == ';'
	}) {
		list = append(list, part)
	}
	return NormalizeSuffixList(list)
}
