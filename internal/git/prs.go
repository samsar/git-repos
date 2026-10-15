package git

import "strings"

func normalizeRepoKey(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "ssh://")
	s = strings.TrimPrefix(s, "git@")
	s = strings.TrimPrefix(s, "git://")

	if idx := strings.Index(s, "github.com:"); idx >= 0 {
		s = s[idx+len("github.com:"):]
	} else if idx := strings.Index(s, "github.com/"); idx >= 0 {
		s = s[idx+len("github.com/"):]
	} else {
		return ""
	}
	s = strings.TrimPrefix(s, "/")
	parts := strings.Split(s, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}
