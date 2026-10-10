package usage

import (
	"sort"
	"time"

	"github.com/muthuishere/huddle-next/internal/artifacts"
	"github.com/muthuishere/huddle-next/internal/config"
	"github.com/muthuishere/huddle-next/internal/eventlog"
	"github.com/muthuishere/huddle-next/internal/gates"
)

// Flow is the SDLC flow view derived from a repo's event log (ADR 20 §4).
type Flow struct {
	Repo            string  `json:"repo"`
	Events          int     `json:"events"`
	GatesRequested  int     `json:"gatesRequested"`
	GatesApproved   int     `json:"gatesApproved"`
	GatesRejected   int     `json:"gatesRejected"`
	GatesPending    int     `json:"gatesPending"`
	GateWaitP50Min  float64 `json:"gateWaitP50Min"` // request -> decision, minutes
	GateWaitP95Min  float64 `json:"gateWaitP95Min"`
	Overrides       int     `json:"overrides"`
	Verdicts        int     `json:"verdicts"`
	VerifiedChanges int     `json:"verifiedChanges"` // artifacts with at least one verdict
	FirstPassPASS   int     `json:"firstPassPass"`   // artifacts whose first verdict was PASS
	FirstPassRate   float64 `json:"firstPassRate"`   // 0..1
	StaleVerdicts   int     `json:"staleVerdicts"`   // latest verdicts no longer matching the tree (needs repo root)
	StaleChecked    bool    `json:"staleChecked"`
}

// FlowMetrics computes flow metrics from the event log only (no repo files).
func FlowMetrics(repoName string) (Flow, error) { return FlowMetricsAt(repoName, "") }

// FlowMetricsAt is FlowMetrics plus the cheap stale-verdict check (HEAD moved
// or artifact file changed) when the repo root is known.
func FlowMetricsAt(repoName, root string) (Flow, error) {
	f := Flow{Repo: repoName}
	evs, err := eventlog.List(repoName, eventlog.Filter{})
	if err != nil {
		return f, err
	}
	f.Events = len(evs)
	requested := map[string]time.Time{}
	decided := map[string]bool{}
	first := map[string]string{}
	var waits []float64
	for _, e := range evs {
		gid, _ := e.Data["gate"].(string)
		switch e.Type {
		case gates.EvRequest:
			if gid == "" {
				continue
			}
			if _, dup := requested[gid]; !dup {
				requested[gid] = e.TS
				f.GatesRequested++
			}
		case gates.EvApproved, gates.EvRejected:
			t0, ok := requested[gid]
			if !ok || decided[gid] {
				continue
			}
			decided[gid] = true
			if e.Type == gates.EvApproved {
				f.GatesApproved++
			} else {
				f.GatesRejected++
			}
			if d := e.TS.Sub(t0); d >= 0 {
				waits = append(waits, d.Minutes())
			}
		case gates.EvOverride:
			f.Overrides++
		case gates.EvVerify:
			f.Verdicts++
			id, _ := e.Data["artifactId"].(string)
			if id == "" && len(e.Refs) > 0 {
				id = e.Refs[0]
			}
			if _, seen := first[id]; id != "" && !seen {
				first[id], _ = e.Data["verdict"].(string)
			}
		}
	}
	f.GatesPending = f.GatesRequested - f.GatesApproved - f.GatesRejected
	sort.Float64s(waits)
	f.GateWaitP50Min = round1(percentile(waits, 50))
	f.GateWaitP95Min = round1(percentile(waits, 95))
	f.VerifiedChanges = len(first)
	for _, v := range first {
		if v == gates.PASS {
			f.FirstPassPASS++
		}
	}
	if f.VerifiedChanges > 0 {
		f.FirstPassRate = float64(f.FirstPassPASS) / float64(f.VerifiedChanges)
	}
	if root != "" && f.Verdicts > 0 {
		cfg, err := config.Load(root)
		if err == nil {
			arts, err := artifacts.Discover(root, cfg)
			if err == nil {
				latest, err := gates.Latest(repoName)
				if err == nil {
					cur := make(map[string]artifacts.Artifact, len(arts))
					for _, a := range arts {
						cur[a.Path] = a
					}
					head := gates.HeadCommit(root)
					for _, v := range latest {
						if gates.CheapStale(v, head, cur) {
							f.StaleVerdicts++
						}
					}
					f.StaleChecked = true
				}
			}
		}
	}
	return f, nil
}

func round1(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }
