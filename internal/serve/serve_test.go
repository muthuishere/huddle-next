package serve

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/hooks"
	"github.com/muthuishere/huddle-next/internal/repo"
)

// testServer builds a Server against an isolated HUDDLE_HOME and a fake repo,
// so no test touches the developer's real state.
func testServer(t *testing.T) *Server {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HUDDLE_HOME", filepath.Join(tmp, "huddle-home"))
	root := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{
		Repo: repo.Info{Root: root, Name: "proj", Branch: "main", IsGit: true},
		// Never run real hooks from a test.
		Fire: func(config.Config, string, string, hooks.Payload) hooks.Result {
			return hooks.Result{Decision: hooks.Allow}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestListenBindsLoopbackOnly is the security property in ADR 20 §1: the
// dashboard must never be reachable off-box.
func TestListenBindsLoopbackOnly(t *testing.T) {
	s := testServer(t)
	ln, err := s.Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	host, _, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("bound %s, want 127.0.0.1 — a non-loopback bind exposes the dashboard", host)
	}
	if origin, _ := s.servedOrigin(); !strings.HasPrefix(origin, "http://127.0.0.1:") {
		t.Fatalf("served origin %q should be fixed to the loopback listener", origin)
	}
}

// TestHostGuardBlocksDNSRebinding covers the ServeHTTP pre-check: a page on
// another domain resolving to 127.0.0.1 still sends its own Host header.
func TestHostGuardBlocksDNSRebinding(t *testing.T) {
	s := testServer(t)
	s.SetOrigin("http://127.0.0.1:16211")

	for _, tc := range []struct {
		host string
		want int
	}{
		{"127.0.0.1:16211", http.StatusOK},
		{"localhost:16211", http.StatusOK},
		{"evil.example", http.StatusForbidden},
		{"evil.example:16211", http.StatusForbidden},
	} {
		r := httptest.NewRequest(http.MethodGet, "/api/gates", nil)
		r.Host = tc.host
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("Host %q: got %d, want %d", tc.host, w.Code, tc.want)
		}
	}
}

// TestMutationGuard checks all three conditions on every write: token, same
// origin, JSON content type. Any one missing must fail closed.
func TestMutationGuard(t *testing.T) {
	s := testServer(t)
	s.SetOrigin("http://127.0.0.1:16211")
	body := `{"type":"learning","text":"hello"}`

	cases := []struct {
		name        string
		token       string
		origin      string
		contentType string
		want        int
	}{
		{"no token", "", "http://127.0.0.1:16211", "application/json", http.StatusForbidden},
		{"wrong token", "deadbeef", "http://127.0.0.1:16211", "application/json", http.StatusForbidden},
		{"no origin", s.Token(), "", "application/json", http.StatusForbidden},
		{"cross origin", s.Token(), "http://evil.example", "application/json", http.StatusForbidden},
		{"wrong content type", s.Token(), "http://127.0.0.1:16211", "text/plain", http.StatusForbidden},
		{"all correct", s.Token(), "http://127.0.0.1:16211", "application/json", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/log", strings.NewReader(body))
			r.Host = "127.0.0.1:16211"
			if tc.token != "" {
				r.Header.Set("X-Huddle-Token", tc.token)
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.contentType != "" {
				r.Header.Set("Content-Type", tc.contentType)
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Errorf("got %d, want %d — body %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

// TestGatesHaveNoMutationRoute is the structural half of ADR 20 §3: the
// dashboard cannot approve a gate because no such route is registered.
func TestGatesHaveNoMutationRoute(t *testing.T) {
	s := testServer(t)
	s.SetOrigin("http://127.0.0.1:16211")
	for _, path := range []string{"/api/gates", "/api/gate/approve", "/api/gates/approve"} {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		r.Host = "127.0.0.1:16211"
		r.Header.Set("X-Huddle-Token", s.Token())
		r.Header.Set("Origin", "http://127.0.0.1:16211")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code == http.StatusOK {
			t.Errorf("POST %s returned 200 — gates must never be decided over HTTP", path)
		}
	}
}

// TestLogRejectsGateTypes keeps the log endpoint from becoming a gate backdoor.
func TestLogRejectsGateTypes(t *testing.T) {
	s := testServer(t)
	s.SetOrigin("http://127.0.0.1:16211")
	for _, typ := range []string{"gate.approve", "verdict", "artifact.written", ""} {
		r := httptest.NewRequest(http.MethodPost, "/api/log",
			strings.NewReader(`{"type":"`+typ+`","text":"x"}`))
		r.Host = "127.0.0.1:16211"
		r.Header.Set("X-Huddle-Token", s.Token())
		r.Header.Set("Origin", "http://127.0.0.1:16211")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("type %q: got %d, want 400", typ, w.Code)
		}
	}
}

// TestIndexInjectsTokenAndCSP guards the served page's contract.
func TestIndexInjectsTokenAndCSP(t *testing.T) {
	s := testServer(t)
	s.SetOrigin("http://127.0.0.1:16211")
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "127.0.0.1:16211"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("index: got %d", w.Code)
	}
	page := w.Body.String()
	if strings.Contains(page, "{{TOKEN}}") {
		t.Error("token placeholder was not substituted")
	}
	if !strings.Contains(page, s.Token()) {
		t.Error("page does not carry the mutation token")
	}
	csp := w.Header().Get("Content-Security-Policy")
	for _, want := range []string{"script-src 'self'", "style-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP missing %q: %s", want, csp)
		}
	}
}

// TestRenderMarkdownIsSafe: input must never become markup, and only
// non-executable href schemes survive.
func TestRenderMarkdownIsSafe(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		reject   []string
		expect   []string
	}{
		{
			name:   "script tag is escaped",
			in:     "<script>alert(1)</script>",
			reject: []string{"<script>"},
			expect: []string{"&lt;script&gt;"},
		},
		{
			name:   "javascript href dropped",
			in:     "[click](javascript:alert(1))",
			reject: []string{"javascript:", "<a href"},
			expect: []string{"click"},
		},
		{
			name:   "data href dropped",
			in:     "[x](data:text/html;base64,abcd)",
			reject: []string{"data:text/html", "<a href"},
		},
		{
			name:   "https href kept",
			in:     "[docs](https://example.com)",
			expect: []string{`<a href="https://example.com"`, "noreferrer"},
		},
		{
			name:   "relative href kept",
			in:     "[spec](intent/x/spec.md)",
			expect: []string{`<a href="intent/x/spec.md"`},
		},
		{
			name:   "img onerror is escaped",
			in:     `<img src=x onerror="alert(1)">`,
			reject: []string{"<img"},
		},
		{
			name:   "fenced code is escaped",
			in:     "```\n<b>raw</b>\n```",
			reject: []string{"<b>raw</b>"},
			expect: []string{"<pre><code>", "&lt;b&gt;"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderMarkdown(tc.in)
			for _, bad := range tc.reject {
				if strings.Contains(got, bad) {
					t.Errorf("output contains %q: %s", bad, got)
				}
			}
			for _, want := range tc.expect {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q: %s", want, got)
				}
			}
		})
	}
}

// TestRenderMarkdownStructure covers the renderer's ordinary job.
func TestRenderMarkdownStructure(t *testing.T) {
	got := RenderMarkdown("# Title\n\n- one\n- two\n\n---\n\npara **bold**\n")
	for _, want := range []string{"<h1>Title</h1>", "<ul>", "<li>one</li>", "<hr>", "<strong>bold</strong>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in: %s", want, got)
		}
	}
}

func TestIsRule(t *testing.T) {
	for _, in := range []string{"---", "***", "___", "- - -", "  ---  "} {
		if !isRule(in) {
			t.Errorf("isRule(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"--", "-*-", "- item", "text", ""} {
		if isRule(in) {
			t.Errorf("isRule(%q) = true, want false", in)
		}
	}
}
