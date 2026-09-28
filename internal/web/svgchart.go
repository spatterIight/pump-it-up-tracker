package web

import (
	"fmt"
	"html/template"
	"math"
	"strconv"
	"strings"

	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
)

// metric is one quantity a chart history can be plotted by.
type metric struct {
	key           string
	label         string
	value         func(*tracker.Play) (float64, bool)
	axis          func(float64) string
	lowerIsBetter bool
	grades        bool
}

var metrics = map[string]metric{
	"score": {
		key:    "score",
		label:  "Score",
		value:  func(p *tracker.Play) (float64, bool) { return float64(p.Score), true },
		axis:   func(v float64) string { return strconv.Itoa(int(math.Round(v/1000))) + "k" },
		grades: true,
	},
	"misses": {
		key:   "misses",
		label: "Misses",
		value: func(p *tracker.Play) (float64, bool) {
			if p.Judgments == nil {
				return 0, false
			}
			return float64(p.Judgments.Misses()), true
		},
		axis:          func(v float64) string { return strconv.Itoa(int(math.Round(v))) },
		lowerIsBetter: true,
	},
	"perfect": {
		key:   "perfect",
		label: "Perfect %",
		value: func(p *tracker.Play) (float64, bool) {
			if p.Judgments == nil {
				return 0, false
			}
			return p.Judgments.PerfectRate(), true
		},
		axis: func(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) + "%" },
	},
}

// layout is the geometry of one rendering of a chart. Text in an SVG scales
// with its viewBox, so each chart is drawn twice, wide and narrow, and CSS
// shows the one that keeps labels readable at the current screen width.
type layout struct {
	name                                string
	w, h, padL, padR, padT, padB, inset float64
	labelGap                            float64
}

var layouts = []layout{
	{name: "wide", w: 900, h: 300, padL: 58, padR: 60, padT: 16, padB: 38, inset: 18, labelGap: 80},
	{name: "narrow", w: 400, h: 250, padL: 44, padR: 46, padT: 14, padB: 34, inset: 12, labelGap: 64},
}

func (l layout) plotW() float64 { return l.w - l.padL - l.padR }
func (l layout) plotH() float64 { return l.h - l.padT - l.padB }

type chartPoint struct {
	x, y   float64
	v      float64
	play   *tracker.Play
	isBest bool
	// failed marks a fail with no result: it has no value, so it is drawn
	// as a marker on the bottom edge rather than as a point of the line.
	failed bool
}

// renderChart plots a chart history by the named metric as SVG.
func renderChart(h *tracker.ChartHistory, key string) template.HTML {
	m, ok := metrics[key]
	if !ok {
		return ""
	}
	var out strings.Builder
	for _, l := range layouts {
		out.WriteString(renderLayout(h, m, l))
	}
	return template.HTML(out.String())
}

func renderLayout(h *tracker.ChartHistory, m metric, l layout) string {
	// slots are the plays along the x axis, in order: pts those with a
	// value, fails those without a result.
	var slots, pts, fails []*chartPoint
	best, haveBest := 0.0, false
	for _, p := range h.Plays {
		if !p.HasScore() {
			cp := &chartPoint{play: p, failed: true}
			slots = append(slots, cp)
			fails = append(fails, cp)
			continue
		}
		v, ok := m.value(p)
		if !ok {
			continue
		}
		cp := &chartPoint{v: v, play: p}
		if m.grades {
			cp.isBest = p.IsPB
		} else if !haveBest || (m.lowerIsBetter && v < best) || (!m.lowerIsBetter && v > best) {
			cp.isBest = true
			best, haveBest = v, true
		}
		slots = append(slots, cp)
		pts = append(pts, cp)
	}
	if len(slots) == 0 {
		return ""
	}

	plotW, plotH := l.plotW(), l.plotH()
	bottom := l.padT + plotH
	lo, hi := 0.0, 1.0
	if len(pts) > 0 {
		lo, hi = domain(m, pts)
	}
	yOf := func(v float64) float64 { return l.padT + (1-(v-lo)/(hi-lo))*plotH }
	for i, p := range slots {
		if len(slots) == 1 {
			p.x = l.padL + plotW/2
		} else {
			p.x = l.padL + l.inset + float64(i)*(plotW-2*l.inset)/float64(len(slots)-1)
		}
		if p.failed {
			p.y = bottom
		} else {
			p.y = yOf(p.v)
		}
	}

	id := "g-" + h.Chart.Key() + "-" + m.key + "-" + l.name
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="graph-svg is-%s" viewBox="0 0 %g %g" role="img" aria-label="%s history for %s">`,
		l.name, l.w, l.h, template.HTMLEscapeString(m.label), template.HTMLEscapeString(h.Chart.String()))
	fmt.Fprintf(&b, `<defs><linearGradient id="%s-line" x1="0" x2="1" y1="0" y2="0"><stop offset="0" class="stop-a"/><stop offset="1" class="stop-b"/></linearGradient>`+
		`<linearGradient id="%s-area" x1="0" x2="0" y1="0" y2="1"><stop offset="0" class="stop-area"/><stop offset="1" class="stop-area-end"/></linearGradient></defs>`, id, id)

	fmt.Fprintf(&b, `<rect class="g-frame" x="%g" y="%g" width="%g" height="%g" rx="10"/>`, l.padL, l.padT, plotW, plotH)

	if len(pts) == 0 {
		// Only fails: there is nothing to put on a y axis.
		fmt.Fprintf(&b, `<text class="g-empty" x="%g" y="%g" text-anchor="middle">Not cleared yet</text>`, l.padL+plotW/2, l.padT+plotH/2+6)
	} else {
		if m.grades {
			writeGradeBands(&b, l, lo, hi, yOf)
		}
		for _, t := range ticks(lo, hi, 4) {
			y := yOf(t)
			if !m.grades {
				fmt.Fprintf(&b, `<line class="g-grid" x1="%g" x2="%g" y1="%.1f" y2="%.1f"/>`, l.padL, l.padL+plotW, y, y)
			}
			fmt.Fprintf(&b, `<text class="g-axis" x="%g" y="%.1f" text-anchor="end">%s</text>`, l.padL-8, y+4.5, m.axis(t))
		}
	}
	writeDates(&b, l, slots)

	if len(pts) > 1 {
		var area, line strings.Builder
		fmt.Fprintf(&area, "M%.1f %.1f", pts[0].x, bottom)
		for i, p := range pts {
			fmt.Fprintf(&area, "L%.1f %.1f", p.x, p.y)
			if i == 0 {
				fmt.Fprintf(&line, "M%.1f %.1f", p.x, p.y)
			} else {
				fmt.Fprintf(&line, "L%.1f %.1f", p.x, p.y)
			}
		}
		fmt.Fprintf(&area, "L%.1f %.1fZ", pts[len(pts)-1].x, bottom)
		fmt.Fprintf(&b, `<path class="g-area" d="%s" fill="url(#%s-area)"/>`, area.String(), id)
		if step := bestStep(m, pts, yOf); step != "" {
			fmt.Fprintf(&b, `<path class="g-best" d="%s"/>`, step)
		}
		fmt.Fprintf(&b, `<path class="g-line" d="%s" stroke="url(#%s-line)"/>`, line.String(), id)
	}

	for _, p := range pts {
		classes := "pt"
		if p.isBest {
			classes += " is-best"
		}
		if p.play.Broken {
			classes += " is-broken"
		}
		tip := tooltip(m, p)
		fmt.Fprintf(&b, `<g class="%s" tabindex="0" data-tip="%s"><title>%s</title>`+
			`<circle class="pt-hit" cx="%.1f" cy="%.1f" r="16"/><circle class="pt-halo" cx="%.1f" cy="%.1f" r="10"/><circle class="pt-dot" cx="%.1f" cy="%.1f" r="5"/></g>`,
			classes, template.HTMLEscapeString(strings.Join(tip, "\n")), template.HTMLEscapeString(strings.Join(tip, " · ")),
			p.x, p.y, p.x, p.y, p.x, p.y)
	}
	for _, p := range fails {
		tip := tooltip(m, p)
		fmt.Fprintf(&b, `<g class="pt is-failed" tabindex="0" data-tip="%s"><title>%s</title>`+
			`<circle class="pt-hit" cx="%.1f" cy="%.1f" r="16"/><circle class="pt-halo" cx="%.1f" cy="%.1f" r="10"/>`+
			`<rect class="pt-fail" x="%.1f" y="%.1f" width="12" height="12" rx="2" transform="rotate(45 %.1f %.1f)"/>`+
			`<path class="pt-dot" d="M%.1f %.1fl7 7m0 -7l-7 7"/></g>`,
			template.HTMLEscapeString(strings.Join(tip, "\n")), template.HTMLEscapeString(strings.Join(tip, " · ")),
			p.x, p.y, p.x, p.y, p.x-6, p.y-6, p.x, p.y, p.x-3.5, p.y-3.5)
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func domain(m metric, pts []*chartPoint) (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, p := range pts {
		lo, hi = math.Min(lo, p.v), math.Max(hi, p.v)
	}
	switch m.key {
	case "score":
		pad := math.Max(4000, (hi-lo)*0.18)
		lo, hi = lo-pad, hi+pad
		if hi-lo < 20000 {
			mid := (hi + lo) / 2
			lo, hi = mid-10000, mid+10000
		}
		if hi > tracker.MaxScore {
			lo, hi = lo-(hi-tracker.MaxScore), tracker.MaxScore
		}
		lo = math.Max(0, lo)
	case "misses":
		lo, hi = 0, math.Max(4, math.Ceil(hi*1.25))
	default:
		lo, hi = math.Max(0, lo-3), math.Min(100, hi+2)
		if hi-lo < 6 {
			lo = math.Max(0, hi-6)
		}
	}
	return lo, hi
}

// ticks returns about n round values spanning lo..hi.
func ticks(lo, hi float64, n int) []float64 {
	span := hi - lo
	if span <= 0 {
		return []float64{lo}
	}
	raw := span / float64(n)
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	step := mag
	for _, f := range []float64{1, 2, 2.5, 5, 10} {
		if f*mag >= raw {
			step = f * mag
			break
		}
	}
	var out []float64
	for v := math.Ceil(lo/step) * step; v <= hi+step*1e-9; v += step {
		out = append(out, v)
	}
	return out
}

func writeGradeBands(b *strings.Builder, l layout, lo, hi float64, yOf func(float64) float64) {
	lastLabel := math.Inf(-1)
	thresholds := tracker.GradeThresholds()
	for i, t := range thresholds {
		min := float64(t.Min)
		if min <= lo || min > hi {
			continue
		}
		top := hi
		if i > 0 {
			top = math.Min(hi, float64(thresholds[i-1].Min))
		}
		y, yTop := yOf(min), yOf(top)
		fmt.Fprintf(b, `<rect class="g-zone tier-%s" x="%g" y="%.1f" width="%g" height="%.1f"/>`, t.Grade.Tier(), l.padL, yTop, l.plotW(), y-yTop)
		fmt.Fprintf(b, `<line class="g-grade-line tier-%s" x1="%g" x2="%g" y1="%.1f" y2="%.1f"/>`, t.Grade.Tier(), l.padL, l.padL+l.plotW(), y, y)
		if y-lastLabel >= 14 {
			fmt.Fprintf(b, `<text class="g-grade-label tier-%s" x="%g" y="%.1f">%s</text>`, t.Grade.Tier(), l.padL+l.plotW()+8, y+4.5, t.Grade)
			lastLabel = y
		}
	}
}

func writeDates(b *strings.Builder, l layout, pts []*chartPoint) {
	multiYear := pts[0].play.Date.Year() != pts[len(pts)-1].play.Date.Year()
	format := "2 Jan"
	if multiYear {
		format = "2 Jan '06"
	}
	lastX := math.Inf(-1)
	for i, p := range pts {
		day := p.play.Date.Format("2006-01-02")
		if i > 0 && day == pts[i-1].play.Date.Format("2006-01-02") {
			continue
		}
		if i > 0 && len(pts) <= 60 {
			x := (pts[i-1].x + p.x) / 2
			fmt.Fprintf(b, `<line class="g-day" x1="%.1f" x2="%.1f" y1="%g" y2="%g"/>`, x, x, l.padT, l.padT+l.plotH())
		}
		if p.x-lastX >= l.labelGap {
			fmt.Fprintf(b, `<text class="g-axis" x="%.1f" y="%g" text-anchor="middle">%s</text>`, p.x, l.padT+l.plotH()+24, p.play.Date.Format(format))
			lastX = p.x
		}
	}
}

// bestStep traces the best value so far as a step line.
func bestStep(m metric, pts []*chartPoint, yOf func(float64) float64) string {
	var d strings.Builder
	best, have := 0.0, false
	for _, p := range pts {
		if m.grades && p.play.Broken {
			if have {
				fmt.Fprintf(&d, "H%.1f", p.x)
			}
			continue
		}
		switch {
		case !have:
			best, have = p.v, true
			fmt.Fprintf(&d, "M%.1f %.1f", p.x, yOf(best))
		case (m.lowerIsBetter && p.v < best) || (!m.lowerIsBetter && p.v > best):
			best = p.v
			fmt.Fprintf(&d, "H%.1fV%.1f", p.x, yOf(best))
		default:
			fmt.Fprintf(&d, "H%.1f", p.x)
		}
	}
	return d.String()
}

func tooltip(m metric, p *chartPoint) []string {
	play := p.play
	when := play.Date.Format("Mon 2 Jan 2006")
	if play.HasTime {
		when += " · " + play.Date.Format("15:04")
	}
	lines := []string{when}
	if !play.HasScore() {
		return append(lines, "Failed")
	}
	switch m.key {
	case "score":
		lines = append(lines, formatInt(play.Score)+" · "+string(play.Grade))
		if play.Plate != "" {
			lines = append(lines, play.Plate.Name())
		}
	case "misses":
		j := play.Judgments
		lines = append(lines, fmt.Sprintf("%d misses (%d bad, %d miss)", j.Misses(), j.Bad, j.Miss))
	case "perfect":
		lines = append(lines, fmt.Sprintf("%.1f%% perfect (%d of %d)", play.Judgments.PerfectRate(), play.Judgments.Perfect, play.Judgments.Notes()))
	}
	switch {
	case play.Broken:
		lines = append(lines, "Stage break")
	case m.grades && play.FirstClear:
		lines = append(lines, "First clear")
	case m.grades && play.IsPB:
		lines = append(lines, "New best "+signed(play.Delta()))
	case !m.grades && p.isBest:
		lines = append(lines, "Best so far")
	}
	return lines
}
