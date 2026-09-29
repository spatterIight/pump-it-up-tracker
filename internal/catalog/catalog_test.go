package catalog

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
)

func version(t *testing.T, tr *tracker.Tracker, id string) *tracker.Version {
	t.Helper()
	v, ok := tr.Version(id)
	if !ok {
		t.Fatalf("no version %s", id)
	}
	return v
}

func chart(t *testing.T, s string) tracker.Chart {
	t.Helper()
	c, err := tracker.ParseChart(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func empty(t *testing.T) *tracker.Tracker {
	t.Helper()
	tr, err := tracker.Load(strings.NewReader(`{"schema_version": 1}`))
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// The built-in lists have every chart of Phoenix and Phoenix 2.
func TestBuiltIn(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	tr := empty(t)
	phoenix, phoenix2 := c.Mix(version(t, tr, "phoenix")), c.Mix(version(t, tr, "phoenix2"))
	if phoenix == nil || phoenix2 == nil || c.Mix(version(t, tr, "prime2")) != nil {
		t.Fatal("wrong lists")
	}
	if len(phoenix.Charts) < 4000 || len(phoenix2.Charts) < 4000 {
		t.Errorf("%d and %d charts", len(phoenix.Charts), len(phoenix2.Charts))
	}
	bd := phoenix.Lookup("big daddy", chart(t, "S11"))
	if bd == nil || bd.Song != "Big Daddy" || bd.TrustedNotes() != 561 || bd.StepArtist == "" || bd.BPM == "" {
		t.Fatalf("Big Daddy S11 = %+v", bd)
	}
	// A chart keeps its ID across versions, even when it is re-rated.
	yog1, yog2 := phoenix.Lookup("Yog-Sothoth", chart(t, "S9")), phoenix2.Lookup("Yog-Sothoth", chart(t, "S10"))
	if yog1 == nil || yog2 == nil || yog1.ID != yog2.ID || c.ChartID("Yog-Sothoth", version(t, tr, "phoenix"), chart(t, "S9")) != yog1.ID {
		t.Errorf("Yog-Sothoth: %+v, %+v", yog1, yog2)
	}
	if levels := phoenix.Levels(tracker.ModeSingle); levels[0] != 1 || levels[len(levels)-1] < 26 {
		t.Errorf("single levels = %v", levels)
	}
	if n := len(phoenix.Folder(tracker.ModeCoOp, 2)); n < 100 {
		t.Errorf("%d co-op x2 charts", n)
	}
}

const sample = "ChartId,Song,Artist,StepArtist,Type,Level,SongType,BPM,NoteCount,Badges,PassDifficulty,ScoreDifficulty,ScoringLevel\n" +
	"id-1,Rock the house,Matduke,FEFEMZ,S,14,Arcade,120 ~ 120,701,Twists; Drills,Hard,Medium,14.6\n" +
	"id-2,Kasou Shinja,MYTH & ROID,EXC,S,14,Arcade,190,1000,,Easy,,14.2\n" +
	"id-3,Break Through Myself,Nanahoshi,BPM,S,14,Arcade,100 ~ 200,1111,,Unrecorded,Easy,13.9\n" +
	"id-4,Destination,SHK,Co-op,CoOp,2,Arcade,145,654,,,,\n" +
	"id-5,Conflict,Siromaru + Cranky,YAHPP,S,14,Arcade,160,785,,,,\n"

func TestMix(t *testing.T) {
	c, err := LoadFS(fstest.MapFS{"phoenix.csv": {Data: []byte("\uFEFF" + sample)}})
	if err != nil {
		t.Fatal(err)
	}
	m := c.Mix(version(t, empty(t), "phoenix"))
	for title, want := range map[string]string{
		"rock the house":                         "Rock the house",
		"Kasou Shinja 仮装信者":                      "Kasou Shinja",
		"Break Through Myself feat. Risa Yuzuki": "Break Through Myself",
		"DESTINATION!":                           "Destination",
		"Destiny":                                "",
	} {
		if got, _ := m.Song(title); got != want {
			t.Errorf("Song(%q) = %q, want %q", title, got, want)
		}
	}
	rock := m.Lookup("Rock the house", chart(t, "S14"))
	if rock == nil || rock.BPM != "120" || !slices.Equal(rock.Skills, []string{"Twists", "Drills"}) || rock.Pass != Hard || rock.Score != Medium || rock.ScoringLevel != 14.6 {
		t.Errorf("Rock the house = %+v", rock)
	}
	if m.Lookup("Rock the house", chart(t, "S15")) != nil || m.Lookup("Destination", chart(t, "CoOp2")) == nil {
		t.Error("wrong charts found")
	}
	if b := m.Lookup("Break Through Myself", chart(t, "S14")); b.BPM != "100–200" || b.Pass != Unknown || b.ClearDifficulty() != Easy || b.TrustedNotes() != 0 {
		t.Errorf("Break Through Myself = %+v", b)
	}
	if k := m.Lookup("Kasou Shinja", chart(t, "S14")); k.Notes != 1000 || k.TrustedNotes() != 0 {
		t.Errorf("a round note count is trusted: %+v", k)
	}
	// The easiest to clear first; a chart nobody knows the difficulty of last.
	folder := m.Folder(tracker.ModeSingle, 14)
	slices.SortStableFunc(folder, Ease)
	var order []string
	for _, ch := range folder {
		order = append(order, ch.Song)
	}
	if strings.Join(order, ", ") != "Break Through Myself, Kasou Shinja, Rock the house, Conflict" {
		t.Errorf("easiest first = %v", order)
	}
}

func TestCheck(t *testing.T) {
	c, err := LoadFS(fstest.MapFS{"phoenix.csv": {Data: []byte(sample)}})
	if err != nil {
		t.Fatal(err)
	}
	tr, err := tracker.Load(strings.NewReader(`{"schema_version": 1, "scores": [
		{"song": "Rock the house", "chart": "S14", "date": "2026-08-01", "judgments": {"perfect": 600, "great": 50, "good": 20, "bad": 10, "miss": 20}, "max_combo": 300},
		{"song": "Rock the house", "chart": "S14", "date": "2026-08-02", "judgments": {"perfect": 600, "great": 50, "good": 20, "bad": 10, "miss": 21}, "max_combo": 300},
		{"song": "Rock the house", "chart": "S14", "date": "2026-08-03", "judgments": {"perfect": 600, "great": 50, "good": 20, "bad": 10, "miss": 22}, "max_combo": 300, "broken": true},
		{"song": "Conflict", "chart": "S14", "date": "2026-08-01", "judgments": {"perfect": 700, "great": 50, "good": 20, "bad": 10, "miss": 6}, "max_combo": 300},
		{"song": "Conflict", "chart": "S14", "date": "2026-08-02", "judgments": {"perfect": 700, "great": 50, "good": 20, "bad": 10, "miss": 6}, "max_combo": 300},
		{"song": "Kasou Shinja", "chart": "S14", "date": "2026-08-01", "judgments": {"perfect": 700, "great": 50, "good": 20, "bad": 10, "miss": 5}, "max_combo": 300},
		{"song": "Destination", "chart": "CoOp2", "date": "2026-08-01", "score": 900000},
		{"song": "Conflict", "chart": "S14", "date": "2026-08-01", "version": "phoenix2", "judgments": {"perfect": 1, "great": 0, "good": 0, "bad": 0, "miss": 0}, "max_combo": 1}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	got := c.Check(tr)
	want := []string{
		"Conflict S14 in Phoenix: PIU Scores lists 785 notes, but the judgments of 2 plays add up to a different number: 2026-08-01 (786), 2026-08-02 (786); they agree with each other, so the list may be wrong",
		"Rock the house S14 in Phoenix: PIU Scores lists 701 notes, but the judgments of the play on 2026-08-01 (700) add up to a different number; check for a typo",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("warnings =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
