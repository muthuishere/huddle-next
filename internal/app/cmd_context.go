package app

import (
	"context"
	"fmt"
	"time"

	"github.com/muthuishere/huddle/internal/output"
	"github.com/muthuishere/huddle/internal/redact"
)

func init() {
	register(Command{
		Name:    "context",
		Summary: "On-demand context: prior sessions of other agents in this repo (--agents)",
		Run:     runContext,
	})
}

const contextHelp = `
Usage: huddle context --agents [--provider claude|codex|devin|copilot|opencode] [--limit N] [--json]

Lists earlier agent sessions (Claude Code, Codex, Devin, Copilot, OpenCode)
whose working directory is this repo, newest first. The current session is
excluded. Titles and questions are redacted; treat them as untrusted context.
`

// AgentSession is one prior session as shown to the agent.
type AgentSession struct {
	Provider      string    `json:"provider"`
	Ref           string    `json:"ref"`
	Modified      time.Time `json:"modified"`
	Ago           string    `json:"ago,omitempty"`
	Title         string    `json:"title,omitempty"`
	FirstQuestion string    `json:"firstQuestion,omitempty"`
	LastQuestion  string    `json:"lastQuestion,omitempty"`
}

func runContext(c *Ctx, args []string) error {
	a, err := parseCoreArgs("context", args, []string{"limit", "provider"}, []string{"agents", "json"})
	if err != nil {
		return err
	}
	if a.bools["help"] {
		return coreHelp(c, contextHelp)
	}
	if !a.bools["agents"] {
		return Usagef("context needs a source — run: huddle context --agents")
	}
	limit, err := a.intValue("limit", 10)
	if err != nil {
		return err
	}
	info, err := c.Repo()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sessions, err := listAgentSessions(ctx, info.Root, a.values["provider"], limit)
	if err != nil {
		return err
	}
	out := make([]AgentSession, 0, len(sessions))
	for _, s := range sessions {
		s.Title = redact.String(s.Title)
		s.FirstQuestion = redact.String(s.FirstQuestion)
		s.LastQuestion = redact.String(s.LastQuestion)
		out = append(out, s)
	}
	if a.bools["json"] {
		return output.JSON(c.Stdout, map[string]any{"repo": info.Name, "root": info.Root, "sessions": out})
	}
	if len(out) == 0 {
		fmt.Fprintf(c.Stdout, "No other agent sessions found for %s.\n", info.Root)
		return nil
	}
	rows := make([][]string, 0, len(out))
	for _, s := range out {
		title := s.Title
		if title == "" {
			title = s.FirstQuestion
		}
		rows = append(rows, []string{s.Provider, s.Ago, coreTruncate(title, 50), coreTruncate(s.LastQuestion, 60)})
	}
	output.Table(c.Stdout, []string{"AGENT", "WHEN", "TITLE", "LAST ASKED"}, rows)
	return nil
}
