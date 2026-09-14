//go:build crossmem

package app

import (
	"context"
	"fmt"

	"github.com/muthuishere/crossmemcli/pkg/crossmem"
)

// listAgentSessions reads other agents' session stores in-process via the
// crossmem library. Built with -tags crossmem (see go.work in AGENTS.md).
func listAgentSessions(ctx context.Context, root, provider string, limit int) ([]AgentSession, error) {
	client, err := crossmem.New(crossmem.Options{})
	if err != nil {
		return nil, fmt.Errorf("crossmem config: %w — check $CROSSMEM_CONFIG", err)
	}
	sessions, err := client.List(ctx, crossmem.ListOptions{CWD: root, Limit: limit, Provider: provider, Questions: true})
	if err != nil {
		return nil, fmt.Errorf("list agent sessions: %w — retry with a narrower --provider", err)
	}
	out := make([]AgentSession, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, AgentSession{
			Provider: s.Provider, Ref: s.Ref, Modified: s.Modified, Ago: s.Ago,
			Title: s.Title, FirstQuestion: s.FirstQuestion, LastQuestion: s.LastQuestion,
		})
	}
	return out, nil
}
