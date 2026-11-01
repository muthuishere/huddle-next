// Package bands is the deterministic control-band detector (ADR 12, 16, 19
// row 11): baseline mean and sample standard deviation, then the latest point
// is tiered by sigma distance — <1σ ok, ≥1σ log, ≥2σ diagnose, ≥3σ breach.
package bands

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/muthuishere/huddle-next/artifacts"
)

// Tiers, in increasing severity.
const (
	TierOK       = "ok"
	TierLog      = "log"
	TierDiagnose = "diagnose"
	TierBreach   = "breach"
)

// Config is a parsed bands.yaml.
type Config struct {
	Metric    string   `json:"metric"`
	Window    int      `json:"window"`           // first N points form the baseline
	Mean      *float64 `json:"mean,omitempty"`   // given baseline
	StdDev    *float64 `json:"stddev,omitempty"` // given baseline
	Direction string   `json:"direction"`        // up | down | both
	MinPoints int      `json:"minPoints"`
	Log       float64  `json:"log"`
	Diagnose  float64  `json:"diagnose"`
	Breach    float64  `json:"breach"`
}

// ParseConfig reads bands.yaml.
func ParseConfig(src []byte) (Config, error) {
	doc, err := artifacts.ParseYAML(src)
	if err != nil {
		return Config{}, fmt.Errorf("bands.yaml: %w", err)
	}
	c := Config{Direction: "both", MinPoints: 3, Log: 1, Diagnose: 2, Breach: 3}
	c.Metric, _ = doc["metric"].(string)
	if n, ok := doc["metric"].(float64); ok {
		c.Metric = strconv.FormatFloat(n, 'f', -1, 64)
	}
	if strings.TrimSpace(c.Metric) == "" {
		return c, errors.New("bands.yaml: metric is required")
	}
	if b, ok := doc["baseline"].(map[string]any); ok {
		if v, ok := b["window"].(float64); ok {
			c.Window = int(v)
		}
		if v, ok := b["mean"].(float64); ok {
			c.Mean = &v
		}
		for _, key := range []string{"stddev", "sigma"} {
			if v, ok := b[key].(float64); ok {
				c.StdDev = &v
			}
		}
	}
	if r, ok := doc["rules"].(map[string]any); ok {
		if v, ok := r["direction"].(string); ok {
			c.Direction = strings.ToLower(v)
		}
		if v, ok := r["min_points"].(float64); ok {
			c.MinPoints = int(v)
		}
	}
	if t, ok := doc["tiers"].(map[string]any); ok {
		for key, dst := range map[string]*float64{"log": &c.Log, "diagnose": &c.Diagnose, "breach": &c.Breach} {
			if v, ok := t[key].(float64); ok {
				*dst = v
			}
		}
	}
	switch c.Direction {
	case "up", "down", "both":
	default:
		return c, fmt.Errorf("bands.yaml: rules.direction must be up, down, or both (got %q)", c.Direction)
	}
	if !(c.Log > 0 && c.Log <= c.Diagnose && c.Diagnose <= c.Breach) {
		return c, errors.New("bands.yaml: tiers must satisfy 0 < log ≤ diagnose ≤ breach")
	}
	if (c.Mean == nil) != (c.StdDev == nil) {
		return c, errors.New("bands.yaml: give both baseline.mean and baseline.stddev, or neither")
	}
	return c, nil
}

// ReadSeries reads one value per line; with commas, the last column is the
// value. Blank lines, "#" comments, and non-numeric header rows are skipped.
func ReadSeries(r io.Reader) ([]float64, error) {
	var out []float64
	sc := bufio.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		t := strings.TrimSpace(sc.Text())
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		cols := strings.Split(t, ",")
		cell := strings.Trim(strings.TrimSpace(cols[len(cols)-1]), `"`)
		v, err := strconv.ParseFloat(cell, 64)
		if err != nil {
			if len(out) == 0 {
				continue // header
			}
			return nil, fmt.Errorf("series line %d: %q is not a number", line, cell)
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("series line %d: value must be finite", line)
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

// Result is one check.
type Result struct {
	Metric    string  `json:"metric"`
	Points    int     `json:"points"`
	Baseline  int     `json:"baselinePoints"`
	Mean      float64 `json:"mean"`
	StdDev    float64 `json:"stddev"`
	Latest    float64 `json:"latest"`
	Sigma     float64 `json:"sigma"` // signed distance in σ
	Tier      string  `json:"tier"`
	Direction string  `json:"direction"`
}

// Check computes the tier of the latest point.
func Check(c Config, series []float64) (Result, error) {
	r := Result{Metric: c.Metric, Points: len(series), Direction: c.Direction}
	if len(series) == 0 {
		return r, errors.New("series is empty")
	}
	r.Latest = series[len(series)-1]
	if c.Mean != nil {
		r.Mean, r.StdDev = *c.Mean, *c.StdDev
	} else {
		n := c.Window
		if n <= 0 || n >= len(series) {
			n = len(series) - 1 // everything before the latest point
		}
		if n < c.MinPoints || n < 2 {
			return r, fmt.Errorf("need at least %d baseline points before the latest, have %d", max(c.MinPoints, 2), n)
		}
		r.Baseline = n
		r.Mean, r.StdDev = meanStd(series[:n])
	}
	switch {
	case r.StdDev > 0:
		r.Sigma = (r.Latest - r.Mean) / r.StdDev
	case r.Latest == r.Mean:
		r.Sigma = 0
	default:
		r.Sigma = math.Copysign(1e9, r.Latest-r.Mean) // zero spread: any change is off the chart (finite for JSON)
	}
	d := r.Sigma
	switch c.Direction {
	case "up":
	case "down":
		d = -d
	default:
		d = math.Abs(d)
	}
	switch {
	case d >= c.Breach:
		r.Tier = TierBreach
	case d >= c.Diagnose:
		r.Tier = TierDiagnose
	case d >= c.Log:
		r.Tier = TierLog
	default:
		r.Tier = TierOK
	}
	r.Mean, r.StdDev, r.Sigma = round(r.Mean), round(r.StdDev), round(r.Sigma)
	return r, nil
}

// meanStd returns the mean and sample standard deviation (n-1).
func meanStd(xs []float64) (float64, float64) {
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	var ss float64
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	return mean, math.Sqrt(ss / float64(len(xs)-1))
}

func round(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

// BreachProblem is the Problem section text for a drafted intent.
func BreachProblem(r Result) string {
	return fmt.Sprintf("`%s` breached its control band: latest value %g is %.2fσ from the baseline mean %g (σ = %g, %d baseline points, direction %s). Source: huddle bands check.",
		r.Metric, r.Latest, r.Sigma, r.Mean, r.StdDev, r.Baseline, r.Direction)
}
