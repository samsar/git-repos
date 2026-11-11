package git

import "testing"

func TestRedactURL(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"no credentials", "https://github.com/acme/api.git", "https://github.com/acme/api.git"},
		{"password", "https://user:ghp_secret@github.com/acme/api.git", "https://***@github.com/acme/api.git"},
		{"token as username", "https://ghp_secret@github.com/acme/api.git", "https://***@github.com/acme/api.git"},
		{"http", "http://user:pass@example.com/acme/api.git", "http://***@example.com/acme/api.git"},
		{"ssh user", "ssh://git@github.com/acme/api.git", "ssh://git@github.com/acme/api.git"},
		{"ssh password", "ssh://user:pass@example.com/acme/api.git", "ssh://user:***@example.com/acme/api.git"},
		{"scp-like", "git@github.com:acme/api.git", "git@github.com:acme/api.git"},
		{"scp-like at in path", "git@github.com:acme/api@v2.git", "git@github.com:acme/api@v2.git"},
		{"scp-like password", "user:pass@example.com:acme/api.git", "***@example.com:acme/api.git"},
		{"scp-like password and at in path", "user:pass@example.com:acme/api@v2.git", "***@example.com:acme/api@v2.git"},
		{"password without a path", "user:pass@host", "***@host"},
		{"password without a path colon", "user:pass@host/path", "***@host/path"},
		{"malformed", "https://user:pass@example.com", "https://***@example.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RedactURL(c.in); got != c.want {
				t.Errorf("RedactURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
