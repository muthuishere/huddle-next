package serve

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/muthuishere/huddle/internal/config"
	"github.com/muthuishere/huddle/internal/paths"
	"github.com/muthuishere/huddle/internal/repo"
)

// httpError carries a status code out of a handler.
type httpError struct {
	code int
	msg  string
}

func (e *httpError) Error() string { return e.msg }

func errStatus(code int, msg string) error { return &httpError{code: code, msg: msg} }

// target is the repo a request is about. Known repos have a root on disk;
// unknown ones only have their state dir, so only the log is readable.
type target struct {
	info  repo.Info
	known bool
}

func (t target) needRoot() error {
	if !t.known {
		return errStatus(http.StatusConflict, "read-only: the root of repo "+t.info.Name+" is unknown — run huddle serve inside it once")
	}
	return nil
}

func (t target) config() (config.Config, error) {
	if !t.known {
		return config.Config{}, nil
	}
	return config.Load(t.info.Root)
}

type getFunc func(t target, r *http.Request) (any, error)
type postFunc func(t target, r *http.Request, body []byte) (any, error)

func (s *Server) resolveTarget(r *http.Request) (target, error) {
	name := r.URL.Query().Get("repo")
	if name == "" || name == s.repo.Name {
		return target{info: s.repo, known: true}, nil
	}
	if root := knownRoots()[name]; root != "" {
		if info, err := repo.Identify(root); err == nil && info.Name == name {
			return target{info: info, known: true}, nil
		}
	}
	if paths.SafeName(name) != name {
		return target{}, errStatus(http.StatusBadRequest, "invalid repo name")
	}
	if st, err := os.Stat(paths.RepoState(name)); err == nil && st.IsDir() {
		return target{info: repo.Info{Name: name}}, nil
	}
	return target{}, errStatus(http.StatusNotFound, "no huddle state for repo "+name+" — see /api/repos")
}

func (s *Server) get(h getFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET")
			writeErr(w, errStatus(http.StatusMethodNotAllowed, "GET only"))
			return
		}
		key := r.URL.Path + "?" + r.URL.Query().Encode()
		if body, ok := s.cache.get(key); ok {
			writeRaw(w, http.StatusOK, body)
			return
		}
		t, err := s.resolveTarget(r)
		if err != nil {
			writeErr(w, err)
			return
		}
		v, err := h(t, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		body, err := json.Marshal(v)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.cache.put(key, body)
		writeRaw(w, http.StatusOK, body)
	})
}

// post guards every mutation: token, same origin, JSON content type.
func (s *Server) post(h postFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			writeErr(w, errStatus(http.StatusMethodNotAllowed, "POST only"))
			return
		}
		if err := s.guard(r); err != nil {
			writeErr(w, err)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeErr(w, errStatus(http.StatusBadRequest, "read body"))
			return
		}
		t, err := s.resolveTarget(r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := t.needRoot(); err != nil {
			writeErr(w, err)
			return
		}
		v, err := h(t, r, body)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.cache.clear()
		out, _ := json.Marshal(v)
		writeRaw(w, http.StatusOK, out)
	})
}

func (s *Server) guard(r *http.Request) error {
	origin, _ := s.servedOrigin()
	tok := r.Header.Get("X-Huddle-Token")
	if tok == "" || !constantEq(tok, s.token) {
		return errStatus(http.StatusForbidden, "missing or wrong X-Huddle-Token")
	}
	if origin == "" || r.Header.Get("Origin") != origin {
		return errStatus(http.StatusForbidden, "Origin must be "+origin)
	}
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return errStatus(http.StatusForbidden, "Content-Type must be application/json")
	}
	return nil
}

func constantEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func writeRaw(w http.ResponseWriter, code int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write(body)
}

func writeErr(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	var he *httpError
	if errors.As(err, &he) {
		code = he.code
	}
	b, _ := json.Marshal(map[string]any{"error": err.Error(), "readOnly": code == http.StatusConflict})
	writeRaw(w, code, b)
}

func decodeBody(body []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errStatus(http.StatusBadRequest, "invalid JSON body: "+err.Error())
	}
	return nil
}

// cache is a tiny TTL cache keyed by path+query; mutations clear it.
type cache struct {
	ttl time.Duration
	mu  sync.Mutex
	m   map[string]cacheEntry
}

type cacheEntry struct {
	body []byte
	at   time.Time
}

func newCache(ttl time.Duration) *cache { return &cache{ttl: ttl, m: map[string]cacheEntry{}} }

func (c *cache) get(key string) ([]byte, bool) {
	if c.ttl < 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || time.Since(e.at) > c.ttl {
		return nil, false
	}
	return e.body, true
}

func (c *cache) put(key string, body []byte) {
	if c.ttl < 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) > 512 {
		c.m = map[string]cacheEntry{}
	}
	c.m[key] = cacheEntry{body: body, at: time.Now()}
}

func (c *cache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = map[string]cacheEntry{}
}

// rootsFile maps repo name -> last root `huddle serve` saw, so the Repos page
// can switch to repos whose root the event log does not record.
func rootsFile() string { return filepath.Join(paths.Global(), "serve-roots.json") }

func knownRoots() map[string]string {
	m := map[string]string{}
	if b, err := os.ReadFile(rootsFile()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func rememberRoot(info repo.Info) {
	if info.Name == "" || info.Root == "" {
		return
	}
	m := knownRoots()
	if m[info.Name] == info.Root {
		return
	}
	m[info.Name] = info.Root
	_ = config.WriteJSON(rootsFile(), m)
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
