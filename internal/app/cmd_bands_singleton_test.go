package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/huddle-next/artifacts"
)

// bands.yaml is one per repo by design (ADR 12 §5, ADR 18): it configures one
// metric, and a second metric lives in its own file that `check --config`
// reads. The defect was the refusal, which said only "edit it by hand".
func TestBandsNewIsOnePerRepoAndSaysSo(t *testing.T) {
	root := artTestEnv(t)
	artMust(t, root, "bands", "new", "checkout-p95-ms")

	_, err := artRun(t, root, "bands", "new", "signup_error_rate")
	if !errors.Is(err, artifacts.ErrExists) {
		t.Fatalf("second bands new: %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"one bands.yaml per repo", "huddle bands check --series", "--config"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal should explain %q:\n%s", want, msg)
		}
	}
	if !strings.Contains(msg, "huddle bands show bands, then edit") {
		t.Errorf("refusal should say how to change the metric:\n%s", msg)
	}

	// The refusal leaves the committed artifact alone.
	body, err := os.ReadFile(filepath.Join(root, "bands.yaml"))
	if err != nil || !strings.Contains(string(body), "checkout-p95-ms") {
		t.Fatalf("bands.yaml changed: %q %v", body, err)
	}

	// The documented escape hatch works: a second metric in its own file.
	second := filepath.Join(root, "ops", "signup_error_rate.yaml")
	if err := os.MkdirAll(filepath.Dir(second), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "metric: signup_error_rate\nbaseline:\n  mean: 1\n  stddev: 1\nrules:\n  min_points: 3\ntiers:\n  log: 1\n  diagnose: 2\n  breach: 3\n"
	if err := os.WriteFile(second, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	series := filepath.Join(root, "series.csv")
	if err := os.WriteFile(series, []byte("1\n1\n1\n1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := artMust(t, root, "bands", "check", "--series", series, "--config", second)
	if !strings.Contains(out, "signup_error_rate") {
		t.Errorf("check --config did not read the second metric:\n%s", out)
	}

	// The help says it too.
	if out := artMust(t, root, "bands", "--help"); !strings.Contains(out, "one per repo") {
		t.Errorf("bands help:\n%s", out)
	}
}
