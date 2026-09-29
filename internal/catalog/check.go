package catalog

import (
	"fmt"
	"strings"

	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
)

// Check returns a warning for each chart with plays whose judgments do not
// add up to the chart's note count. Those are likely typos, but PIU Scores'
// counts can be wrong too, so they do not stop the data from loading. A
// stage break's judgments stop part-way, so it is not checked.
func (c *Catalog) Check(t *tracker.Tracker) []string {
	var out []string
	for _, song := range t.Songs {
		for _, h := range song.Charts {
			listed := c.Notes(song.Title, h.Version, h.Chart)
			if listed == 0 {
				continue
			}
			var dates []string
			agree := true
			first := -1
			for _, p := range h.Plays {
				if p.Judgments == nil || p.Broken {
					continue
				}
				n := p.Judgments.Notes()
				if first < 0 {
					first = n
				}
				agree = agree && n == first
				if n != listed {
					dates = append(dates, fmt.Sprintf("%s (%d)", p.Date.Format("2006-01-02"), n))
				}
			}
			if len(dates) == 0 {
				continue
			}
			w := fmt.Sprintf("%s %s in %s: PIU Scores lists %d notes, but the judgments of the play on %s add up to a different number; check for a typo",
				song.Title, h.Chart, h.Version.Name, listed, dates[0])
			if len(dates) > 1 {
				w = fmt.Sprintf("%s %s in %s: PIU Scores lists %d notes, but the judgments of %d plays add up to a different number: %s",
					song.Title, h.Chart, h.Version.Name, listed, len(dates), strings.Join(dates, ", "))
				if agree {
					w += "; they agree with each other, so the list may be wrong"
				} else {
					w += "; check for a typo"
				}
			}
			out = append(out, w)
		}
	}
	return out
}

// Notes returns the note count of chart c of a song in version v, 0 when the
// catalog does not have it or it looks like a placeholder.
func (c *Catalog) Notes(song string, v *tracker.Version, chart tracker.Chart) int {
	if ch := c.Mix(v).Lookup(song, chart); ch != nil {
		return ch.TrustedNotes()
	}
	return 0
}
