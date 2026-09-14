// Package redact strips secrets from any text before huddle stores it or shows
// it to an agent. It is the single choke point (ADR 8, 19).
package redact

import (
	"regexp"
	"strings"
)

// Mask replaces every detected secret value.
const Mask = "[REDACTED]"

// rule is one pattern. When keep > 0 the first keep submatches are preserved
// (e.g. the "password=" prefix) and only the rest of the match is masked.
type rule struct {
	re   *regexp.Regexp
	keep int
}

// Compiled once at package init; String does no compilation.
var rules = []rule{
	// Private key blocks (PEM, OpenSSH, PGP).
	{re: regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY( BLOCK)?-----[\s\S]*?-----END [A-Z0-9 ]*PRIVATE KEY( BLOCK)?-----`)},
	// JSON Web Tokens.
	{re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)},
	// Authorization headers: Bearer / Basic / token.
	{re: regexp.MustCompile(`(?i)(\b(?:bearer|basic)\s+)[A-Za-z0-9._~+/=-]{12,}`), keep: 1},
	// AWS access key ids.
	{re: regexp.MustCompile(`\b(?:AKIA|ASIA|ABIA|ACCA|AGPA|AIDA|AROA|ANPA|ANVA|AIPA)[A-Z0-9]{16}\b`)},
	// GitHub tokens.
	{re: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,255}\b`)},
	{re: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,255}\b`)},
	// GitLab personal access tokens.
	{re: regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`)},
	// Slack tokens and webhooks.
	{re: regexp.MustCompile(`\bxox[abprseo]-[A-Za-z0-9-]{10,}\b`)},
	{re: regexp.MustCompile(`https://hooks\.slack\.com/services/[A-Za-z0-9/_-]+`)},
	// Anthropic and OpenAI keys (sk-ant-…, sk-proj-…, sk-…).
	{re: regexp.MustCompile(`\bsk-(?:ant-|proj-|svcacct-|admin-)?[A-Za-z0-9_-]{20,}\b`)},
	// Google API keys.
	{re: regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	// Stripe keys.
	{re: regexp.MustCompile(`\b(?:sk|rk|pk)_(?:live|test)_[A-Za-z0-9]{16,}\b`)},
	// npm tokens.
	{re: regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`)},
	// Credentials embedded in URLs: scheme://user:pass@host.
	{re: regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^\s:/@]+:)[^\s@/]+(@)`), keep: 2},
	// AWS secret access key assignments.
	{re: regexp.MustCompile(`(?i)(aws_?secret_?access_?key["']?\s*[:=]\s*["']?)[A-Za-z0-9/+=]{40}`), keep: 1},
	// key-like names assigned a value: password=…, token: …, "secret": "…".
	{re: regexp.MustCompile(`(?i)(\b[A-Za-z0-9_.-]*(?:passw(?:or)?d|pwd|passphrase|secret|token|api[_-]?key|apikey|access[_-]?key|private[_-]?key|client[_-]?secret|credentials?)[A-Za-z0-9_.-]*["']?\s*[:=]\s*["']?)([^\s"',;&]{4,})`), keep: 1},
	// Generic high-entropy value (32+ base64/hex) after a key-like name.
	{re: regexp.MustCompile(`(?i)(\b(?:key|sig|signature|hash|session|cookie|salt|nonce)[A-Za-z0-9_.-]*["']?\s*[:=]\s*["']?)([A-Za-z0-9+/=_-]{32,})`), keep: 1},
}

// String returns s with secret-looking values replaced by [REDACTED].
func String(s string) string {
	if s == "" {
		return s
	}
	for _, r := range rules {
		if r.keep == 0 {
			s = r.re.ReplaceAllString(s, Mask)
			continue
		}
		s = r.re.ReplaceAllStringFunc(s, func(m string) string {
			sub := r.re.FindStringSubmatch(m)
			if len(sub) <= r.keep {
				return Mask
			}
			if r.keep == 2 { // prefix … suffix (URL credentials)
				return sub[1] + Mask + sub[2]
			}
			if strings.Contains(sub[1], Mask) || strings.HasPrefix(m[len(sub[1]):], Mask) {
				return m // already masked by an earlier rule
			}
			return sub[1] + Mask
		})
	}
	return s
}

// Any redacts every string inside a decoded JSON value (maps, slices, strings).
func Any(v any) any {
	switch t := v.(type) {
	case string:
		return String(t)
	case map[string]any:
		for k, x := range t {
			t[k] = Any(x)
		}
		return t
	case []any:
		for i, x := range t {
			t[i] = Any(x)
		}
		return t
	default:
		return v
	}
}
