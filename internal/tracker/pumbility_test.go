package tracker

import (
	"fmt"
	"strings"
	"testing"
)

func TestPhoenixPumbility(t *testing.T) {
	tr := load(t, `{"schema_version": 1, "scores": [
		{"song": "Nemesis", "chart": "S16", "date": "2026-08-01", "score": 838531},
		{"song": "Conflict", "chart": "S15", "date": "2026-08-01", "score": 880430},
		{"song": "Conflict", "chart": "S15", "date": "2026-08-02", "score": 912850},
		{"song": "Conflict", "chart": "S15", "date": "2026-08-03", "score": 990000, "broken": true},
		{"song": "Conflict", "chart": "D13", "date": "2026-08-02", "score": 868984},
		{"song": "Vook", "chart": "S10", "date": "2026-08-03", "score": 969192},
		{"song": "Katkoi", "chart": "S9", "date": "2026-08-03", "score": 978502},
		{"song": "Katkoi", "chart": "CoOp2", "date": "2026-08-03", "score": 978502},
		{"song": "Katkoi", "chart": "SP12", "date": "2026-08-03", "score": 978502},
		{"song": "DUEL", "chart": "S13", "date": "2026-08-03", "broken": true},
		{"song": "Vook", "chart": "S12", "date": "2026-08-03", "score": 800000, "version": "prime2"}
	]}`)
	phoenix, _ := tr.Version("phoenix")
	p := tr.Pumbility(phoenix)
	var got []string
	for _, rc := range p.Charts {
		got = append(got, fmt.Sprintf("%s %s %s %g", rc.History.Song.Title, rc.History.Chart, rc.Grade, rc.Value))
	}
	// Plays below level 10, co-op and performance charts, fails and other
	// versions' plays do not count; a stage break with a score is not a clear.
	want := "Nemesis S16 A+ 279|Conflict S15 AA 250|Conflict D13 A+ 144|Vook S10 AAA+ 115"
	if strings.Join(got, "|") != want {
		t.Errorf("charts = %q, want %q", got, want)
	}
	if p.Total != 788 || p.Full() || p.Cut() != 0 || !p.Charts[3].InPool {
		t.Errorf("total = %g, full %v, cut %g", p.Total, p.Full(), p.Cut())
	}
	// A+ to AA on a level 16 adds a tenth of 310.
	if n := p.Charts[0]; n.Next != "AA" || n.NextMin != 900000 || n.Gain != 31 {
		t.Errorf("Nemesis next = %s from %d, +%g", n.Next, n.NextMin, n.Gain)
	}
	var history []string
	for _, pt := range p.History {
		history = append(history, fmt.Sprintf("%s %g", pt.Date.Format("01-02"), pt.Total))
	}
	if strings.Join(history, " ") != "08-01 504 08-02 673 08-03 788" {
		t.Errorf("history = %v", history)
	}
	if got := p.TotalOn(mustDate(t, "2026-08-02")); got != 673 {
		t.Errorf("total on 2 Aug = %g", got)
	}
	if got := p.TotalOn(mustDate(t, "2026-07-31")); got != 0 {
		t.Errorf("total before the first play = %g", got)
	}
	if prime2, _ := tr.Version("prime2"); tr.Pumbility(prime2) != nil {
		t.Error("Prime 2 has a PUMBILITY")
	}
}

// Only the fifty most valuable charts count.
func TestPumbilityPool(t *testing.T) {
	var scores []string
	for i := range 52 {
		// 52 level 12 AAs worth 130, but one of them is only an A (104) and
		// one an AA+ (136.5).
		score := 910000
		switch i {
		case 0:
			score = 800000
		case 1:
			score = 930000
		}
		scores = append(scores, fmt.Sprintf(`{"song": "Song %d", "chart": "S12", "date": "2026-08-01", "score": %d}`, i, score))
	}
	tr := load(t, `{"schema_version": 1, "scores": [`+strings.Join(scores, ",")+`]}`)
	phoenix, _ := tr.Version("phoenix")
	p := tr.Pumbility(phoenix)
	if len(p.Charts) != 52 || len(p.Pool()) != 50 || !p.Full() || p.Cut() != 130 {
		t.Fatalf("%d charts, pool %d, cut %g", len(p.Charts), len(p.Pool()), p.Cut())
	}
	// 136.5 + 49 × 130; the A and one of the AAs are left out.
	if p.Total != 6506.5 || p.Charts[0].Value != 136.5 || p.Charts[51].Value != 104 || p.Charts[51].InPool || p.Charts[50].InPool {
		t.Errorf("total = %g", p.Total)
	}
	// An AA left out only adds what AA+ is worth over the cut.
	if rc := p.Charts[50]; rc.Next != "AA+" || rc.Gain != 6.5 {
		t.Errorf("an AA outside the pool: next %s, +%g", rc.Next, rc.Gain)
	}
	// An A would get into the pool as an A+ only if that beat the cut: 117 does not.
	if rc := p.Charts[51]; rc.Next != "A+" || rc.Gain != 0 {
		t.Errorf("the A: next %s, +%g", rc.Next, rc.Gain)
	}
	if p.Gain(250) != 120 || p.Gain(100) != 0 {
		t.Errorf("gain of 250 = %g, of 100 = %g", p.Gain(250), p.Gain(100))
	}
}

// Phoenix 2 prices the charts of real breakdowns of PUMBILITY, as PIU Scores
// read them off the official site, to the cent.
func TestPhoenix2Pumbility(t *testing.T) {
	cases := []struct {
		play  string
		value float64
	}{
		{`"chart": "S12", "score": 480000, "grade": "F", "plate": "MG"`, 176.67},
		{`"chart": "D10", "score": 490000, "plate": "SG"`, 181.44},
		{`"chart": "D25", "score": 910000, "plate": "FG"`, 351.52},
		{`"chart": "D24", "score": 930000, "plate": "RG"`, 342.50},
		{`"chart": "D24", "score": 850000, "plate": "MG"`, 326.50},
		{`"chart": "D26", "score": 850000, "plate": "FG"`, 351.54},
		{`"chart": "D10", "score": 650000, "grade": "C", "plate": "MG"`, 217.08},
		{`"chart": "D10", "score": 550000, "plate": "MG"`, 199.08},
		{`"chart": "D10", "score": 750000, "plate": "EG"`, 227.16},
		// A single is priced as a double one level up, and its AA a notch lower.
		{`"chart": "S17", "score": 930000`, 299.2},
		{`"chart": "S24", "score": 999000, "plate": "UG"`, 394.42},
		{`"chart": "S9", "score": 999000, "plate": "PG"`, 0},
	}
	for _, c := range cases {
		tr := load(t, `{"schema_version": 1, "scores": [{"song": "Vook", "date": "2026-10-01", "version": "phoenix2", `+c.play+`}]}`)
		p := tr.Pumbility(tr.Current)
		got := 0.0
		if len(p.Charts) > 0 {
			got = p.Charts[0].Value
		}
		if got != c.value || p.Total != c.value {
			t.Errorf("{%s} is worth %g, want %g", c.play, got, c.value)
		}
	}
	// A grade Phoenix 2 cannot work out is estimated, and the next one up is
	// the lowest it can.
	tr := load(t, `{"schema_version": 1, "scores": [{"song": "Vook", "chart": "D10", "date": "2026-10-01", "version": "phoenix2", "score": 750000}]}`)
	rc := tr.Pumbility(tr.Current).Charts[0]
	if rc.Best.Grade != "" || rc.Grade != "B" || !rc.Estimated || rc.Next != "A" || rc.NextMin != 800000 || rc.Gain != 9 {
		t.Errorf("an ungraded play = %+v", rc)
	}
	// So is the next one up from a grade logged under every cutoff.
	tr = load(t, `{"schema_version": 1, "scores": [{"song": "Vook", "chart": "D10", "date": "2026-10-01", "version": "phoenix2", "score": 750000, "grade": "B"}]}`)
	rc = tr.Pumbility(tr.Current).Charts[0]
	if rc.Grade != "B" || rc.Estimated || rc.Next != "A" || rc.NextMin != 800000 || rc.Gain != 9 {
		t.Errorf("a play logged as B = %+v", rc)
	}
}
