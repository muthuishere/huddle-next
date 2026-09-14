package bands

import (
	"strings"
	"testing"
)

func TestCheckDeterministicTiers(t *testing.T) {
	c, err := ParseConfig([]byte("metric: latency\nbaseline:\n  window: 4\nrules:\n  direction: both\n"))
	if err != nil {
		t.Fatal(err)
	}
	base := []float64{10, 12, 8, 10} // mean 10, sample σ ≈ 1.633
	cases := map[float64]string{10.5: TierOK, 11.7: TierLog, 13.5: TierDiagnose, 15: TierBreach, 4: TierBreach}
	for last, want := range cases {
		r, err := Check(c, append(append([]float64{}, base...), last))
		if err != nil || r.Tier != want {
			t.Errorf("latest %g: tier %s want %s (%+v, %v)", last, r.Tier, want, r, err)
		}
	}
	mean, sd := 100.0, 0.0
	c.Mean, c.StdDev = &mean, &sd
	if r, _ := Check(c, []float64{101}); r.Tier != TierBreach {
		t.Errorf("zero spread change must breach: %+v", r)
	}
	if _, err := ParseConfig([]byte("metric: x\ntiers:\n  log: 3\n  diagnose: 2\n")); err == nil {
		t.Error("unordered tiers accepted")
	}
}

func TestReadSeries(t *testing.T) {
	s, err := ReadSeries(strings.NewReader("ts,value\n# c\n1,2.5\n2,3\n"))
	if err != nil || len(s) != 2 || s[0] != 2.5 {
		t.Fatalf("%v %v", s, err)
	}
	if _, err := ReadSeries(strings.NewReader("1\nabc\n")); err == nil {
		t.Fatal("non-number accepted")
	}
}
