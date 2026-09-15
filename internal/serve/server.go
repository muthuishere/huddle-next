// Package serve is the local dashboard behind `huddle serve` (ADR 20). It binds
// loopback only, reads the same packages the CLI uses, and exposes a small set
// of token-guarded, audited mutations. Gates are never decided here.
package serve

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/muthuishere/huddle/internal/artifacts"
	"github.com/muthuishere/huddle/internal/hooks"
	"github.com/muthuishere/huddle/internal/repo"
	"github.com/muthuishere/huddle/web"
)

// DefaultPort is the dashboard's well-known port.
const DefaultPort = 16211

// By is the actor recorded on every event the dashboard writes.
const By = "serve"

// Options configure a Server.
type Options struct {
	Repo  repo.Info // the repo the dashboard opens on
	Token string    // mutation token; generated when empty
	Fire  artifacts.FireFunc
	// CacheTTL for GET responses; 0 means the 2s default, negative disables.
	CacheTTL time.Duration
	Now      func() time.Time
}

// Server is the dashboard HTTP handler.
type Server struct {
	repo   repo.Info
	token  string
	fire   artifacts.FireFunc
	now    func() time.Time
	cache  *cache
	mux    *http.ServeMux
	mu     sync.RWMutex
	origin string // served origin, e.g. http://127.0.0.1:16211
	host   string // host:port accepted in the Host header
}

// New builds a Server. The web UI is embedded; nothing is fetched at runtime.
func New(o Options) (*Server, error) {
	if o.Token == "" {
		t, err := NewToken()
		if err != nil {
			return nil, err
		}
		o.Token = t
	}
	if o.Fire == nil {
		o.Fire = hooks.Fire
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	ttl := o.CacheTTL
	if ttl == 0 {
		ttl = 2 * time.Second
	}
	s := &Server{repo: o.Repo, token: o.Token, fire: o.Fire, now: o.Now, cache: newCache(ttl), mux: http.NewServeMux()}
	rememberRoot(o.Repo)
	s.routes()
	return s, nil
}

// Token returns the per-process mutation token.
func (s *Server) Token() string { return s.token }

// NewToken returns 32 random hex bytes.
func NewToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// ErrPortInUse is returned by Listen when the port is taken.
var ErrPortInUse = errors.New("port in use")

// Listen binds 127.0.0.1:port (0 = any free port) and fixes the served origin.
func (s *Server) Listen(port int) (net.Listener, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) || strings.Contains(err.Error(), "address already in use") ||
			strings.Contains(strings.ToLower(err.Error()), "only one usage of each socket address") {
			return nil, fmt.Errorf("%w: port %d in use — run: huddle serve --port 0", ErrPortInUse, port)
		}
		return nil, err
	}
	s.SetOrigin("http://" + ln.Addr().String())
	return ln, nil
}

// SetOrigin fixes the origin mutations must come from (tests use httptest's URL).
func (s *Server) SetOrigin(origin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.origin = strings.TrimRight(origin, "/")
	s.host = strings.TrimPrefix(s.origin, "http://")
}

func (s *Server) servedOrigin() (origin, host string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.origin, s.host
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	// DNS rebinding: a page on evil.example resolving to 127.0.0.1 still sends its own Host.
	if _, host := s.servedOrigin(); host != "" && !hostAllowed(r.Host, host) {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func hostAllowed(got, served string) bool {
	if got == served {
		return true
	}
	_, port, err := net.SplitHostPort(served)
	return err == nil && got == "localhost:"+port
}

func (s *Server) routes() {
	sub, _ := fs.Sub(web.FS, ".")
	static := http.FileServer(http.FS(sub))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			s.index(w, r)
			return
		}
		if r.URL.Path == "/app.js" || r.URL.Path == "/app.css" {
			w.Header().Set("Cache-Control", "no-cache")
			static.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
	for path, h := range map[string]getFunc{
		"/api/overview":      s.apiOverview,
		"/api/artifacts":     s.apiArtifacts,
		"/api/artifact":      s.apiArtifact,
		"/api/gates":         s.apiGates,
		"/api/events":        s.apiEvents,
		"/api/personas":      s.apiPersonas,
		"/api/persona/style": s.apiPersonaStyle,
		"/api/hooks":         s.apiHooks,
		"/api/analytics":     s.apiAnalytics,
		"/api/repos":         s.apiRepos,
	} {
		s.mux.Handle(path, s.get(h))
	}
	for path, h := range map[string]postFunc{
		"/api/artifact/new": s.postArtifactNew,
		"/api/lint":         s.postLint,
		"/api/log":          s.postLog,
		"/api/hooks/test":   s.postHooksTest,
	} {
		s.mux.Handle(path, s.post(h))
	}
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	b, err := fs.ReadFile(web.FS, "index.html")
	if err != nil {
		http.Error(w, "ui missing", http.StatusInternalServerError)
		return
	}
	page := strings.Replace(string(b), "{{TOKEN}}", html.EscapeString(s.token), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	_, _ = w.Write([]byte(page))
}
