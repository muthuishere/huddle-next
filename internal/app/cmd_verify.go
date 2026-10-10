package app

import (
	"fmt"
	"sort"

	"github.com/muthuishere/huddle-next/internal/artifacts"
	"github.com/muthuishere/huddle-next/internal/gates"
	"github.com/muthuishere/huddle-next/internal/output"
)

func init() {
	register(Command{Name: "verify", Summary: "Record PASS/CONCERNS/FAIL verdicts stamped with a tree fingerprint", Run: runVerify})
}

const verifyHelp = `Usage: huddle verify <verb> [options]

  record <artifact-id> --verdict PASS|CONCERNS|FAIL [--findings file.json|-]
         [--eval-status passed|unverified|blocked|failed]
                          PASS with an unverified or blocked eval runner is recorded as CONCERNS
  status [<artifact-id>] [--json]   latest verdicts; STALE when the tree fingerprint changed
`

func runVerify(c *Ctx, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(c.Stdout, verifyHelp)
		return nil
	}
	switch args[0] {
	case "record":
		return verifyRecord(c, args[1:])
	case "status":
		return verifyStatus(c, args[1:])
	}
	return Usagef("unknown verb %q — run: huddle verify --help", args[0])
}

func verifyRecord(c *Ctx, args []string) error {
	a, err := artifacts.ParseArgs(args, nil, []string{"verdict", "findings", "eval-status"})
	if err != nil {
		return Usagef("%v", err)
	}
	if len(a.Pos) != 1 || a.Flags["verdict"] == "" {
		return Usagef("huddle verify record <artifact-id> --verdict PASS|CONCERNS|FAIL")
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	v, err := gates.Record(env.Repo.Root, env, gates.RecordOptions{
		Ref: a.Pos[0], Verdict: a.Flags["verdict"], FindingsFile: a.Flags["findings"],
		EvalStatus: a.Flags["eval-status"], By: artifacts.CurrentUser(env.Cfg),
	})
	if err != nil {
		return err
	}
	if a.Bools["json"] {
		return output.JSON(c.Stdout, v)
	}
	output.KV(c.Stdout, "VERDICT", v.Verdict, "ARTIFACT", v.ArtifactID, "FINGERPRINT", v.Fingerprint[:12]+" ("+v.Mode+")")
	if v.Note != "" {
		output.KV(c.Stdout, "NOTE", v.Note)
	}
	return nil
}

func verifyStatus(c *Ctx, args []string) error {
	a, err := artifacts.ParseArgs(args, nil, nil)
	if err != nil {
		return Usagef("%v", err)
	}
	env, err := artEnv(c)
	if err != nil {
		return err
	}
	latest, err := gates.Latest(env.Repo.Name)
	if err != nil {
		return err
	}
	if len(a.Pos) > 0 {
		x, err := artifacts.Resolve(env.Repo.Root, env.Cfg, "", a.Pos[0])
		if err != nil {
			return err
		}
		v, ok := latest[x.ID]
		latest = map[string]gates.Verdict{}
		if ok {
			latest[x.ID] = v
		}
	}
	if len(latest) > 0 {
		fp, _, err := gates.Fingerprint(env.Repo.Root, env.Repo, env.Cfg)
		if err != nil {
			return err
		}
		gates.MarkStale(latest, fp)
	}
	list := make([]gates.Verdict, 0, len(latest))
	for _, v := range latest {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ArtifactID < list[j].ArtifactID })
	if a.Bools["json"] {
		return output.JSON(c.Stdout, list)
	}
	if len(list) == 0 {
		fmt.Fprintln(c.Stdout, "no verdicts — run: huddle verify record <artifact-id> --verdict PASS|CONCERNS|FAIL")
		return nil
	}
	rows := make([][]string, len(list))
	for i, v := range list {
		fresh := "fresh"
		if v.Stale {
			fresh = "STALE"
		}
		rows[i] = []string{v.ArtifactID, v.Verdict, fresh, artDash(v.EvalStatus), v.TS.Local().Format("2006-01-02 15:04"), artDash(v.By)}
	}
	output.Table(c.Stdout, []string{"ARTIFACT", "VERDICT", "TREE", "EVALS", "RECORDED", "BY"}, rows)
	return nil
}
