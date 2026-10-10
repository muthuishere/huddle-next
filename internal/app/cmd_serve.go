package app

import (
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/muthuishere/huddle-next/internal/repo"
	"github.com/muthuishere/huddle-next/internal/serve"
)

func init() {
	register(Command{
		Name:    "serve",
		Summary: "Local dashboard on 127.0.0.1:16211 — status, artifacts, gates, timeline, analytics",
		Run:     runServe,
	})
}

const serveHelp = `Usage: huddle serve [--port 16211] [--open] [--repo <path>]

Starts the dashboard on 127.0.0.1 only — never 0.0.0.0. The UI is embedded in
the binary, so it works offline with no CDN and no build step.

  --port <n>    port to bind; 0 picks a free one and prints it (default 16211)
  --open        open the dashboard in the default browser
  --repo <path> serve a different repo than the working directory

Reads are open to anything that can reach the port; every mutation needs the
per-process token printed below and a same-origin request. Gates are never
approved here — the dashboard shows the terminal command instead, because a
localhost request cannot prove a human made it (ADR 19, ADR 20 §3).
`

func runServe(c *Ctx, args []string) error {
	a, err := parseCoreArgs("serve", args, []string{"port", "repo"}, []string{"open"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, serveHelp)
	}

	port := serve.DefaultPort
	if v := strings.TrimSpace(a.values["port"]); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 65535 {
			return Usagef("--port must be 0-65535, got %q", v)
		}
		port = n
	}

	info, err := c.Repo()
	if err != nil {
		return err
	}
	if root := strings.TrimSpace(a.values["repo"]); root != "" {
		if info, err = repo.Identify(root); err != nil {
			return fmt.Errorf("--repo %s: %w", root, err)
		}
	}

	srv, err := serve.New(serve.Options{Repo: info})
	if err != nil {
		return err
	}

	ln, err := srv.Listen(port)
	if err != nil {
		if errors.Is(err, serve.ErrPortInUse) {
			return fmt.Errorf("port %d is already in use — run: huddle serve --port 0", port)
		}
		return err
	}
	defer ln.Close()

	url := "http://" + ln.Addr().String()
	fmt.Fprintf(c.Stdout, "Huddle dashboard  %s\n", url)
	fmt.Fprintf(c.Stdout, "Repo              %s (%s)\n", info.Name, info.Branch)
	fmt.Fprintf(c.Stdout, "Token             %s\n", srv.Token())
	fmt.Fprintln(c.Stdout, "Loopback only. Ctrl-C to stop.")

	if a.bools["open"] {
		openBrowser(url)
	}

	hs := &http.Server{
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// openBrowser is best-effort; a failure to open is never a failure to serve.
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	_ = exec.Command(cmd, append(args, url)...).Start()
}
