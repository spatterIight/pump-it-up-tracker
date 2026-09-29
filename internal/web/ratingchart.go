package web

import (
	"fmt"
	"html/template"
	"math"
	"strings"

	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
)

// renderRatingChart plots PUMBILITY at the end of each day played, in the
// style of the song page's progress charts: drawn twice, wide and narrow.
func renderRatingChart(p *tracker.Pumbility) template.HTML {
	if p == nil || len(p.History) == 0 {
		return ""
	}
	var out strings.Builder
	for _, l := range layouts {
		out.WriteString(renderRatingLayout(p, l))
	}
	return template.HTML(out.String())
}

func renderRatingLayout(p *tracker.Pumbility, l layout) string {
	pts := p.History
	plotW, plotH := l.plotW(), l.plotH()
	bottom := l.padT + plotH
	hi := 0.0
	for _, pt := range pts {
		hi = math.Max(hi, pt.Total)
	}
	lo := 0.0
	hi = math.Max(100, hi*1.12)
	yOf := func(v float64) float64 { return l.padT + (1-(v-lo)/(hi-lo))*plotH }
	xOf := func(i int) float64 {
		if len(pts) == 1 {
			return l.padL + plotW/2
		}
		return l.padL + l.inset + float64(i)*(plotW-2*l.inset)/float64(len(pts)-1)
	}

	id := "rating-" + p.Version.ID + "-" + l.name
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="graph-svg is-%s" viewBox="0 0 %g %g" role="img" aria-label="PUMBILITY history in %s">`,
		l.name, l.w, l.h, template.HTMLEscapeString(p.Version.Name))
	fmt.Fprintf(&b, `<defs><linearGradient id="%s-line" x1="0" x2="1" y1="0" y2="0"><stop offset="0" class="stop-a"/><stop offset="1" class="stop-b"/></linearGradient>`+
		`<linearGradient id="%s-area" x1="0" x2="0" y1="0" y2="1"><stop offset="0" class="stop-area"/><stop offset="1" class="stop-area-end"/></linearGradient></defs>`, id, id)
	fmt.Fprintf(&b, `<rect class="g-frame" x="%g" y="%g" width="%g" height="%g" rx="10"/>`, l.padL, l.padT, plotW, plotH)
	for _, t := range ticks(lo, hi, 4) {
		y := yOf(t)
		fmt.Fprintf(&b, `<line class="g-grid" x1="%g" x2="%g" y1="%.1f" y2="%.1f"/>`, l.padL, l.padL+plotW, y, y)
		fmt.Fprintf(&b, `<text class="g-axis" x="%g" y="%.1f" text-anchor="end">%s</text>`, l.padL-8, y+4.5, compact(t))
	}
	multiYear := pts[0].Date.Year() != pts[len(pts)-1].Date.Year()
	format := "2 Jan"
	if multiYear {
		format = "2 Jan '06"
	}
	lastX := math.Inf(-1)
	for i, pt := range pts {
		if x := xOf(i); x-lastX >= l.labelGap {
			fmt.Fprintf(&b, `<text class="g-axis" x="%.1f" y="%g" text-anchor="middle">%s</text>`, x, bottom+24, pt.Date.Format(format))
			lastX = x
		}
	}
	if len(pts) > 1 {
		var area, line strings.Builder
		fmt.Fprintf(&area, "M%.1f %.1f", xOf(0), bottom)
		for i, pt := range pts {
			fmt.Fprintf(&area, "L%.1f %.1f", xOf(i), yOf(pt.Total))
			if i == 0 {
				fmt.Fprintf(&line, "M%.1f %.1f", xOf(i), yOf(pt.Total))
			} else {
				fmt.Fprintf(&line, "L%.1f %.1f", xOf(i), yOf(pt.Total))
			}
		}
		fmt.Fprintf(&area, "L%.1f %.1fZ", xOf(len(pts)-1), bottom)
		fmt.Fprintf(&b, `<path class="g-area" d="%s" fill="url(#%s-area)"/>`, area.String(), id)
		fmt.Fprintf(&b, `<path class="g-line" d="%s" stroke="url(#%s-line)"/>`, line.String(), id)
	}
	prev := 0.0
	for i, pt := range pts {
		tip := []string{pt.Date.Format("Mon 2 Jan 2006"), formatRating(pt.Total)}
		classes := "pt"
		if gain := pt.Total - prev; gain > 0.005 {
			tip = append(tip, "+"+formatRating(gain))
			classes += " is-best"
		} else {
			tip = append(tip, "No change")
		}
		prev = pt.Total
		x, y := xOf(i), yOf(pt.Total)
		fmt.Fprintf(&b, `<g class="%s" tabindex="0" data-tip="%s"><title>%s</title>`+
			`<circle class="pt-hit" cx="%.1f" cy="%.1f" r="16"/><circle class="pt-halo" cx="%.1f" cy="%.1f" r="10"/><circle class="pt-dot" cx="%.1f" cy="%.1f" r="5"/></g>`,
			classes, template.HTMLEscapeString(strings.Join(tip, "\n")), template.HTMLEscapeString(strings.Join(tip, " · ")),
			x, y, x, y, x, y)
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// compact writes an axis value briefly: 950, 1.5k, 12k.
func compact(v float64) string {
	switch {
	case v >= 10000:
		return fmt.Sprintf("%.0fk", v/1000)
	case v >= 1000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", v/1000), ".0") + "k"
	}
	return fmt.Sprintf("%.0f", v)
}
