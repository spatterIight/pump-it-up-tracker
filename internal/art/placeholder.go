package art

import (
	"fmt"
	"hash/fnv"
	"html"
	"strings"
	"unicode/utf8"
)

// Width and Height are the proportions of PIU jacket art (700×393).
const (
	Width  = 700
	Height = 393
)

// ArrowPath draws a pad arrow pointing up-left inside a 100×100 box. Mirror
// it for the other corners.
const ArrowPath = "M8 8H66L48 26L92 70L70 92L26 48L8 66Z"

// Placeholder renders stand-in jacket art for a song: a gradient derived from
// the title, the five-panel pad, and the title itself.
func Placeholder(title string) []byte {
	h := fnv.New32a()
	h.Write([]byte(title))
	sum := h.Sum32()
	hue1 := int(sum % 360)
	hue2 := (hue1 + 40 + int(sum>>9)%80) % 360

	lines := wrap(title, 16, 3)
	longest := 1
	for _, l := range lines {
		longest = max(longest, utf8.RuneCountInString(l))
	}
	size := min(86, int(560/(float64(longest)*0.62)))
	lineHeight := float64(size) * 1.05
	top := float64(Height)/2 - lineHeight*float64(len(lines)-1)/2 + float64(size)*0.35

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`, Width, Height, Width, Height)
	fmt.Fprintf(&b, `<defs><linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">`+
		`<stop offset="0" stop-color="hsl(%d 70%% 16%%)"/><stop offset="1" stop-color="hsl(%d 80%% 34%%)"/></linearGradient>`+
		`<radialGradient id="glow" cx="0.8" cy="0.1" r="0.9"><stop offset="0" stop-color="hsl(%d 100%% 70%%)" stop-opacity="0.35"/>`+
		`<stop offset="1" stop-color="hsl(%d 100%% 70%%)" stop-opacity="0"/></radialGradient></defs>`, hue1, hue2, hue2, hue2)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="url(#bg)"/><rect width="%d" height="%d" fill="url(#glow)"/>`, Width, Height, Width, Height)
	b.WriteString(padEmblem(`transform="translate(470 150) scale(2.6) rotate(-12 50 50)"`, 0.13))
	b.WriteString(`<g font-family="'Chakra Petch', 'Segoe UI', system-ui, sans-serif" font-weight="800" font-style="italic" fill="#fff" text-anchor="middle">`)
	for i, l := range lines {
		fmt.Fprintf(&b, `<text x="%d" y="%.1f" font-size="%d" letter-spacing="1">%s</text>`, Width/2, top+float64(i)*lineHeight, size, html.EscapeString(l))
	}
	b.WriteString(`</g></svg>`)
	return []byte(b.String())
}

// padEmblem draws the five panels of a PIU pad in a 100×100 box: four
// diagonal arrows around a centre square.
func padEmblem(attrs string, opacity float64) string {
	return fmt.Sprintf(`<g %s opacity="%.2f" fill="#fff">`+
		`<path d="%[3]s" transform="scale(0.34)"/>`+
		`<path d="%[3]s" transform="translate(100 0) scale(-0.34 0.34)"/>`+
		`<path d="%[3]s" transform="translate(0 100) scale(0.34 -0.34)"/>`+
		`<path d="%[3]s" transform="translate(100 100) scale(-0.34 -0.34)"/>`+
		`<rect x="36" y="36" width="28" height="28" rx="4"/></g>`, attrs, opacity, ArrowPath)
}

// wrap breaks text into at most maxLines lines of about width runes.
func wrap(text string, width, maxLines int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{"?"}
	}
	var lines []string
	cur := ""
	for _, w := range words {
		switch {
		case cur == "":
			cur = w
		case utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(w) <= width:
			cur += " " + w
		default:
			lines = append(lines, cur)
			cur = w
		}
	}
	lines = append(lines, cur)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] += "…"
	}
	return lines
}
