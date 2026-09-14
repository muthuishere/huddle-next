package app

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestDoctor(t *testing.T) {
	root := coreEnv(t)
	t.Setenv("CLAUDE_SKILLS_DIR", t.TempDir())
	out, err := runCore(t, root, "doctor", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	mustWrite(t, filepath.Join(root, ".huddle", "config.json"), `{"hooks":[{"on":"e","run":"definitely-not-a-command-xyz --flag"},{"on":"e","run":".huddle/hooks/missing.sh"}]}`)
	out, err = runCore(t, root, "doctor", "--json")
	if err == nil {
		t.Fatal("missing hook commands should fail doctor")
	}
	var res struct {
		OK     bool          `json:"ok"`
		Checks []DoctorCheck `json:"checks"`
	}
	if jerr := json.Unmarshal([]byte(out), &res); jerr != nil {
		t.Fatal(jerr, out)
	}
	status := map[string]string{}
	for _, c := range res.Checks {
		status[c.Name] = c.Status
	}
	if res.OK || status["hooks"] != "fail" || status["config-repo"] != "ok" || status["global-root"] != "ok" || status["skills-claude"] != "warn" {
		t.Fatalf("%+v", status)
	}
	mustWrite(t, filepath.Join(root, ".huddle", "config.json"), `{"hooks": [`)
	out, _ = runCore(t, root, "doctor", "--json")
	json.Unmarshal([]byte(out), &res)
	for _, c := range res.Checks {
		if c.Name == "config-repo" && c.Status != "fail" {
			t.Fatalf("invalid config not flagged: %+v", c)
		}
	}
	if !hookResolvable("echo hi", root) || hookResolvable("", root) {
		t.Fatal("hookResolvable")
	}
}
