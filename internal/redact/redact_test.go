package redact

import (
	"strings"
	"testing"
)

// All values below are fabricated fakes shaped like real credentials.
func TestStringRedactsSecrets(t *testing.T) {
	cases := map[string]struct{ in, secret string }{
		"aws key id":     {"id AKIAABCDEFGHIJKLMNOP here", "AKIAABCDEFGHIJKLMNOP"},
		"aws secret":     {"aws_secret_access_key = abcdEFGHijklMNOPqrstUVWXyz0123456789ABCD", "abcdEFGHijklMNOPqrstUVWXyz0123456789ABCD"},
		"github ghp":     {"GH ghp_" + strings.Repeat("a1B2", 9) + " ok", "ghp_" + strings.Repeat("a1B2", 9)},
		"github gho":     {"gho_" + strings.Repeat("Z9", 18), "gho_" + strings.Repeat("Z9", 18)},
		"github pat":     {"github_pat_" + strings.Repeat("x", 30), "github_pat_" + strings.Repeat("x", 30)},
		"slack":          {"xoxb-1234567890-abcdefghij", "xoxb-1234567890-abcdefghij"},
		"openai":         {"key sk-proj-" + strings.Repeat("q", 30), "sk-proj-" + strings.Repeat("q", 30)},
		"anthropic":      {"sk-ant-api03-" + strings.Repeat("F", 40), "sk-ant-api03-" + strings.Repeat("F", 40)},
		"google":         {"AIza" + strings.Repeat("k", 35), "AIza" + strings.Repeat("k", 35)},
		"jwt":            {"t eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.c2lnbmF0dXJlZmFrZQ end", "eyJhbGciOiJIUzI1NiJ9"},
		"private key":    {"-----BEGIN RSA PRIVATE KEY-----\nMIIfake\nlines\n-----END RSA PRIVATE KEY-----", "MIIfake"},
		"password":       {"password=hunter2hunter2", "hunter2hunter2"},
		"token colon":    {`token: "abcd1234efgh"`, "abcd1234efgh"},
		"json secret":    {`{"client_secret": "shh-very-secret"}`, "shh-very-secret"},
		"bearer":         {"Authorization: Bearer abc.def.ghi-jkl_mno", "abc.def.ghi-jkl_mno"},
		"url creds":      {"postgres://admin:s3cr3tpw@db.local/app", "s3cr3tpw"},
		"high entropy":   {"session_key=" + strings.Repeat("0f", 20), strings.Repeat("0f", 20)},
		"env style":      {"MY_API_KEY=fakefakefake123", "fakefakefake123"},
		"stripe":         {"sk_live_" + strings.Repeat("9", 24), "sk_live_" + strings.Repeat("9", 24)},
		"multi in block": {"a AKIAABCDEFGHIJKLMNOP b xoxp-1111111111-zzzz", "xoxp-1111111111-zzzz"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			out := String(c.in)
			if strings.Contains(out, c.secret) {
				t.Fatalf("secret survived: %q", out)
			}
			if !strings.Contains(out, Mask) {
				t.Fatalf("no mask in %q", out)
			}
		})
	}
}

func TestStringKeepsOrdinaryText(t *testing.T) {
	for _, s := range []string{
		"",
		"run go test ./... then commit",
		"the token bucket refills every second",
		"see https://github.com/muthuishere/huddle-next/pull/7",
		"sha 6f869ed docs: add MIT LICENSE",
		"author: Muthukumaran",
	} {
		if got := String(s); got != s {
			t.Errorf("changed %q -> %q", s, got)
		}
	}
}

func TestStringKeepsKeyName(t *testing.T) {
	got := String("password=hunter2hunter2")
	if got != "password="+Mask {
		t.Fatalf("got %q", got)
	}
}

func TestAny(t *testing.T) {
	v := map[string]any{"a": []any{"password=abcdefgh", 3.0}, "b": "plain"}
	Any(v)
	if s := v["a"].([]any)[0].(string); strings.Contains(s, "abcdefgh") {
		t.Fatal(s)
	}
	if v["b"] != "plain" {
		t.Fatal(v["b"])
	}
}

func BenchmarkString(b *testing.B) {
	s := strings.Repeat("ordinary log line with nothing secret in it ", 50)
	for i := 0; i < b.N; i++ {
		String(s)
	}
}
