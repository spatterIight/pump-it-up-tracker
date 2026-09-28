package web

import (
	"fmt"
	"html/template"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
)

func (s *Server) funcs() template.FuncMap {
	return template.FuncMap{
		"url":     func(p string) string { return s.base + p },
		"asset":   func(p string) string { return s.base + "/static/" + p + "?v=" + s.assetVersion },
		"artURL":  func(slug string) string { return s.base + "/art/" + slug + "?v=" + s.opts.Art.Version(slug) },
		"songURL": func(slug string) string { return s.base + "/song/" + slug },
		"score":   formatInt,
		"signed":  signed,
		"date":    func(t time.Time) string { return t.Format("Mon 2 Jan 2006") },
		"longDate": func(t time.Time) string {
			return t.Format("Monday 2 January 2006")
		},
		"shortDate": func(t time.Time) string { return t.Format("2 Jan") },
		"clock":     func(t time.Time) string { return t.Format("15:04") },
		"iso":       func(t time.Time) string { return t.Format("2006-01-02T15:04") },
		"unix":      func(t time.Time) int64 { return t.Unix() },
		"ago":       func(t time.Time) string { return ago(t, s.opts.Now()) },
		"pct":       func(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) + "%" },
		"kcal":      func(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) },
		"plural": func(n int, one, many string) string {
			if n == 1 {
				return "1 " + one
			}
			return formatInt(n) + " " + many
		},
		"modes": func(charts []*tracker.ChartHistory) string {
			seen := map[string]bool{}
			var out []string
			for _, h := range charts {
				c := h.Chart.Mode.Class()
				if !seen[c] {
					seen[c] = true
					out = append(out, c)
				}
			}
			return strings.Join(out, " ")
		},
		"search":      func(s *tracker.Song) string { return strings.ToLower(s.Title + " " + s.Artist) },
		"reverse":     reversePlays,
		"take":        func(n int, ps []*tracker.Play) []*tracker.Play { return ps[:min(n, len(ps))] },
		"judgmentBar": judgmentBar,
		"chart":       renderChart,
		"hasJudgments": func(l *tracker.Lineage) bool {
			for _, p := range l.Plays {
				if p.Judgments != nil {
					return true
				}
			}
			return false
		},
		"legend": chartLegend,
		// old reports whether a version is older than the newest one played,
		// which is what gets a version badge.
		"old":         func(v *tracker.Version) bool { return v != s.opts.Tracker.Current },
		"chartsLabel": chartsLabel,
		// chartNames names charts without their versions: "S7 → S8".
		"chartNames": func(hs []*tracker.ChartHistory) string {
			names := make([]string, len(hs))
			for i, h := range hs {
				names[i] = h.Chart.String()
			}
			return strings.Join(names, " → ")
		},
		// statRows counts the stat cards beside a chart's result, which the
		// result spans.
		"statRows": func(l *tracker.Lineage) int {
			n := len(l.Records)
			if l.FewestMisses >= 0 {
				n++
			}
			if l.BestPerfectRate >= 0 {
				n++
			}
			return max(n, 3)
		},
		// aliases are the keys of a lineage's older charts, which open its tab.
		"aliases": func(l *tracker.Lineage) string {
			var keys []string
			for _, h := range l.Charts[:len(l.Charts)-1] {
				keys = append(keys, h.Key())
			}
			return strings.Join(keys, " ")
		},
		"olderRecords": func(l *tracker.Lineage) []*tracker.Record { return l.Records[:len(l.Records)-1] },
		"versionNames": func(r *tracker.Record) string {
			var names []string
			for _, v := range r.Versions() {
				names = append(names, v.Name)
			}
			return strings.Join(names, " and ")
		},
		// filterURL links a list page narrowed to a version, nil for all.
		"filterURL": func(nav string, v *tracker.Version) string {
			u, frag := s.base+"/", "#songs"
			if nav == "activity" {
				u, frag = s.base+"/activity", ""
			}
			if v != nil {
				u += "?version=" + v.ID
			}
			return u + frag
		},
		"gradeShare": func(n, total int) string {
			if total == 0 {
				return "0"
			}
			return strconv.FormatFloat(float64(n)*100/float64(total), 'f', 2, 64)
		},
		"gradeClass": func(g tracker.Grade) string {
			return strings.NewReplacer("+", "p").Replace(strings.ToLower(string(g)))
		},
		"plateClass": func(p tracker.Plate) string { return strings.ToLower(string(p)) },
		"padEmblem":  padEmblem,
	}
}

func formatInt(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "−" + b.String()
	}
	return b.String()
}

func signed(n int) string {
	if n > 0 {
		return "+" + formatInt(n)
	}
	if n == 0 {
		return "±0"
	}
	return formatInt(n)
}

func ago(t, now time.Time) string {
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	days := int(math.Round(time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC).Sub(time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)).Hours() / 24))
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days < 14:
		return fmt.Sprintf("%d days ago", days)
	case days < 60:
		return fmt.Sprintf("%d weeks ago", days/7)
	default:
		return t.Format("2 Jan 2006")
	}
}

func reversePlays(ps []*tracker.Play) []*tracker.Play {
	out := make([]*tracker.Play, len(ps))
	for i, p := range ps {
		out[len(ps)-1-i] = p
	}
	return out
}

// legend says which kinds of marks a chart history's graph has.
type legend struct {
	Scores, Breaks, Failed bool
}

func chartLegend(lin *tracker.Lineage) legend {
	var l legend
	for _, p := range lin.Plays {
		switch {
		case !p.HasScore():
			l.Failed = true
		case p.Broken:
			l.Scores, l.Breaks = true, true
		default:
			l.Scores = true
		}
	}
	return l
}

// judgmentBar draws the judgment counts as one stacked bar.
func judgmentBar(j *tracker.Judgments) template.HTML {
	if j == nil || j.Notes() == 0 {
		return ""
	}
	parts := []struct {
		class string
		n     int
	}{{"perfect", j.Perfect}, {"great", j.Great}, {"good", j.Good}, {"bad", j.Bad}, {"miss", j.Miss}}
	var b strings.Builder
	b.WriteString(`<svg class="jbar" viewBox="0 0 1000 10" preserveAspectRatio="none" aria-hidden="true">`)
	x := 0.0
	for _, p := range parts {
		if p.n == 0 {
			continue
		}
		w := float64(p.n) * 1000 / float64(j.Notes())
		fmt.Fprintf(&b, `<rect class="j-%s" x="%.2f" y="0" width="%.2f" height="10"/>`, p.class, x, w)
		x += w
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// padEmblem draws the five PIU pad panels in the game's colours.
func padEmblem(class string) template.HTML {
	const arrow = "M8 8H66L48 26L92 70L70 92L26 48L8 66Z"
	return template.HTML(fmt.Sprintf(`<svg class="%s" viewBox="0 0 100 100" aria-hidden="true">`+
		`<path class="pad-up" d="%[2]s" transform="scale(0.36)"/>`+
		`<path class="pad-up" d="%[2]s" transform="translate(100 0) scale(-0.36 0.36)"/>`+
		`<path class="pad-down" d="%[2]s" transform="translate(0 100) scale(0.36 -0.36)"/>`+
		`<path class="pad-down" d="%[2]s" transform="translate(100 100) scale(-0.36 -0.36)"/>`+
		`<rect class="pad-center" x="35" y="35" width="30" height="30" rx="5"/></svg>`, template.HTMLEscapeString(class), arrow))
}
