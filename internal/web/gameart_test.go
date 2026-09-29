package web

import (
	"strings"
	"testing"

	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
)

func version(t *testing.T, id string) *tracker.Version {
	t.Helper()
	for _, v := range tracker.Versions() {
		if v.ID == id {
			return &v
		}
	}
	t.Fatalf("no version %s", id)
	return nil
}

// Every grade of every version and every plate has the game's art, since
// there is no drawn fallback for them.
func TestGameArtCoversGradesAndPlates(t *testing.T) {
	s, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range tracker.Versions() {
		for _, g := range v.Scoring.Grades() {
			if s.gradeArt(g, false) == "" || s.gradeArt(g, true) == "" {
				t.Errorf("%s grade %s has no art", v.Name, g)
			}
		}
	}
	// Best first, so ranks count down to 1: a plate added to the tracker
	// shifts them until it is listed here.
	plates := []tracker.Plate{"PG", "UG", "EG", "SG", "MG", "TG", "FG", "RG"}
	for i, p := range plates {
		if p.Rank() != len(plates)-i {
			t.Errorf("plate %s has rank %d, want %d", p, p.Rank(), len(plates)-i)
		}
		if s.plateArt(p) == "" {
			t.Errorf("plate %s has no art", p)
		}
	}
	if s.gradeArt("", false) != "" || s.plateArt("") != "" {
		t.Error("art for no grade or no plate")
	}
}

func TestBallArt(t *testing.T) {
	s, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	phoenix, phoenix2, prime2, xx := version(t, "phoenix"), version(t, "phoenix2"), version(t, "prime2"), version(t, "xx")
	for _, tc := range []struct {
		v     *tracker.Version
		chart string
		want  string
	}{
		{phoenix, "S1", "piu/difficulty/Phoenix/s1.png"},
		{phoenix, "D29", "piu/difficulty/Phoenix/d29.png"},
		{phoenix, "CoOp5", "piu/difficulty/Phoenix/coop5.png"},
		{phoenix2, "S26", "piu/difficulty/Phoenix2/s26.png"},
		{phoenix2, "D29", "piu/difficulty/Phoenix2/d29.png"},
		{phoenix2, "CoOp2", "piu/difficulty/Phoenix2/coop2.png"},
		{phoenix2, "DP24", "piu/difficulty/dp24.png"},
		// Performance charts have the same balls in every mix.
		{phoenix, "SP12", "piu/difficulty/sp12.png"},
		{phoenix, "DP28", "piu/difficulty/dp28.png"},
		{prime2, "S17", "piu/difficulty/s17.png"},
		{xx, "CoOp2", "piu/difficulty/coop2.png"},
		// Past the levels there is art for, the ball is drawn.
		{phoenix, "S29", ""},
		{xx, "D30", ""},
	} {
		c, err := tracker.ParseChart(tc.chart)
		if err != nil {
			t.Fatal(err)
		}
		if got := s.ballArt(tc.v, c); got != tc.want {
			t.Errorf("%s %s: ball %q, want %q", tc.v.Name, tc.chart, got, tc.want)
		}
	}
}

func TestGameArtOnPages(t *testing.T) {
	tr, err := tracker.Load(strings.NewReader(`{"schema_version": 1, "scores": [
		{"song": "Vook", "chart": "S10", "date": "2026-08-01", "score": 969192, "plate": "SG"},
		{"song": "Vook", "chart": "S10", "date": "2026-08-02", "score": 700000, "broken": true},
		{"song": "Vook", "chart": "S30", "date": "2026-08-03", "score": 900000},
		{"song": "Vook", "chart": "D20", "date": "2026-08-04", "score": 800000},
		{"song": "Katkoi", "chart": "S7", "date": "2026-07-30", "version": "prime2", "score": 646600, "grade": "A"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	h := serve(t, tr, "/piu")
	_, activity := get(t, h, "/piu/activity")
	wantAll(t, "activity", activity,
		`<img class="ball" src="/piu/static/piu/difficulty/Phoenix/s10.png?v=`, `alt="Single 10" title="Single 10">`,
		`<img class="ball" src="/piu/static/piu/difficulty/s7.png?v=`,
		`<span class="ball m-single" title="Single 30">30</span>`,
		`<img class="grade" src="/piu/static/piu/letters/aaaplus.png?v=`, `alt="AAA&#43;">`,
		// A stage break shows its grade cracked, as the result screen does.
		`<img class="grade" src="/piu/static/piu/letters/b_broken.png?v=`, `alt="B (stage break)">`,
		`<img class="grade" src="/piu/static/piu/letters/a.png?v=`,
		`<img class="plate" src="/piu/static/piu/plates/sg.png?v=`, `alt="Superb Game" title="Superb Game">`)

	_, song := get(t, h, "/piu/song/vook")
	wantAll(t, "Vook", song, `<span class="plate-full"><img class="plate" src="/piu/static/piu/plates/sg.png?v=`, `<span class="plate-name">Superb Game</span>`)

	_, home := get(t, h, "/piu/")
	i := strings.Index(home, `<span class="stat-value stat-balls">`)
	if i < 0 {
		t.Fatal("no hardest clears")
	}
	hardest := home[i : i+strings.Index(home[i:], "</span>\n")]
	wantAll(t, "hardest clears", hardest, `<span class="ball m-single" title="Single 30">30</span>`,
		`<img class="ball" src="/piu/static/piu/difficulty/Phoenix/d20.png?v=`)

	resp, _ := get(t, h, "/piu/static/piu/COPYRIGHT.txt")
	if resp.StatusCode != 200 {
		t.Errorf("the art's copyright notice is not served: status %d", resp.StatusCode)
	}
}
