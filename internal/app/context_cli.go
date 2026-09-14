//go:build !crossmem

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
)

// listAgentSessions shells out to the crossmem CLI when the binary was built
// without the crossmem library. Context is optional: a missing CLI is a hint,
// not a failure of the huddle.
func listAgentSessions(ctx context.Context, root, provider string, limit int) ([]AgentSession, error) {
	bin, err := exec.LookPath("crossmem")
	if err != nil {
		return nil, errors.New("crossmem is not installed — run: npm i -g @muthuishere/crossmem@latest")
	}
	args := []string{"list", root, "--json", "--limit", strconv.Itoa(limit)}
	if provider != "" {
		args = append(args, "--provider", provider)
	}
	raw, err := exec.CommandContext(ctx, bin, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("crossmem list: %w — run: crossmem list %s", err, root)
	}
	var sessions []AgentSession
	if err := json.Unmarshal(raw, &sessions); err != nil {
		return nil, fmt.Errorf("crossmem list output: %w — upgrade: npm i -g @muthuishere/crossmem@latest", err)
	}
	return sessions, nil
}
