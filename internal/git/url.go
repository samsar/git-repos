package git

import (
	"net/url"
	"strings"
)

// RedactURL masks credentials in a remote URL for display and JSON. The
// userinfo of an HTTP(S) URL can be a token or a password, so it is
// replaced; SSH's git@host:path has only a username and is left as it is.
func RedactURL(raw string) string {
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		// scp-like user@host:path, or user:password@host:path
		if at := strings.LastIndex(raw, "@"); at > 0 {
			if strings.Contains(raw[:at], ":") {
				return "***@" + raw[at+1:]
			}
		}
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		// Redact conservatively when a URL can't be parsed.
		if i := strings.Index(raw, "://"); i >= 0 {
			rest := raw[i+3:]
			if at := strings.Index(rest, "@"); at >= 0 && !strings.Contains(rest[:at], "/") {
				return raw[:i+3] + "***@" + rest[at+1:]
			}
		}
		return raw
	}
	if u.User != nil {
		switch u.Scheme {
		case "http", "https":
			return withUserinfo(u, "***")
		default:
			if _, hasPassword := u.User.Password(); hasPassword {
				return withUserinfo(u, url.User(u.User.Username()).String()+":***")
			}
		}
	}
	return u.String()
}

// withUserinfo is u with userinfo in place of its own; url.URL escapes *
// and other characters a mask uses, so the mask is put back after String.
func withUserinfo(u *url.URL, userinfo string) string {
	u.User = nil
	s := u.String()
	if i := strings.Index(s, "://"); i >= 0 {
		return s[:i+3] + userinfo + "@" + s[i+3:]
	}
	return s
}
