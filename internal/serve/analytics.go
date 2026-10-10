// Analytics view (ADR 20 §4). Everything here is read from the local usage log
// and the repo event log; nothing is fetched and nothing is sent.
package serve

import (
	"net/http"

	"github.com/muthuishere/huddle-next/internal/usage"
)

// defaultAnalyticsSince matches `huddle stats` (ADR 20 §4).
const defaultAnalyticsSince = "30d"

// apiAnalytics serves the same numbers as `huddle stats`, so the dashboard and
// the CLI can never disagree (ADR 20 §2, §4).
func (s *Server) apiAnalytics(t target, r *http.Request) (any, error) {
	sinceArg := r.URL.Query().Get("since")
	if sinceArg == "" {
		sinceArg = defaultAnalyticsSince
	}
	since, err := usage.ParseSince(sinceArg, s.now())
	if err != nil {
		return nil, errStatus(http.StatusBadRequest, err.Error())
	}

	// An explicit ?allRepos=1 aggregates across every repo; otherwise the
	// analytics are scoped to the target repo, as `huddle stats` defaults.
	name := t.info.Name
	if r.URL.Query().Get("allRepos") != "" {
		name = ""
	}

	sum, err := usage.Aggregate(since, name)
	if err != nil {
		return nil, err
	}

	out := map[string]any{
		"repo":    name,
		"since":   sinceArg,
		"usage":   sum,
		"enabled": !usage.Disabled(t.info.Root),
	}
	if t.info.Name != "" {
		if f, err := usage.FlowMetricsAt(t.info.Name, t.info.Root); err == nil {
			out["flow"] = f
		}
	}
	return out, nil
}
