package web

import (
	"strings"

	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
)

// The game's own grade letters, plates and level balls are served from
// static/piu. They are © Andamiro; static/piu/COPYRIGHT.txt says where they
// come from. Their paths are those of the PIU Scores image host.

// gradeArt returns the static path of a grade's letters, cracked for a stage
// break, or "" when there are none.
func (s *Server) gradeArt(g tracker.Grade, broken bool) string {
	if g == "" {
		return ""
	}
	name := strings.ToLower(strings.ReplaceAll(string(g), "+", "plus"))
	if broken {
		name += "_broken"
	}
	return s.staticFile("piu/letters/" + name + ".png")
}

// plateArt returns the static path of a plate, or "" when there is none.
func (s *Server) plateArt(p tracker.Plate) string {
	if p == "" {
		return ""
	}
	return s.staticFile("piu/plates/" + strings.ToLower(string(p)) + ".png")
}

// ballArt returns the static path of a chart's level ball in version v, or ""
// when there is none. A version's own balls are in a folder named after it
// without spaces (difficulty/Phoenix); the others are the balls of XX and
// older mixes, which also serve the performance charts of every version.
func (s *Server) ballArt(v *tracker.Version, c tracker.Chart) string {
	key := c.Key() + ".png"
	if p := s.staticFile("piu/difficulty/" + strings.ReplaceAll(v.Name, " ", "") + "/" + key); p != "" {
		return p
	}
	return s.staticFile("piu/difficulty/" + key)
}

// staticFile returns p if it is a static file, "" otherwise.
func (s *Server) staticFile(p string) string {
	if s.staticFiles[p] {
		return p
	}
	return ""
}

// ballRef is what the ball template draws: a chart in a version.
type ballRef struct {
	Version *tracker.Version
	Chart   tracker.Chart
}

// hardestBalls are the level balls of the hardest single and double cleared
// in the version the stats cover.
func hardestBalls(st tracker.Stats) []ballRef {
	var out []ballRef
	if st.HighestSingle > 0 {
		out = append(out, ballRef{st.Version, tracker.Chart{Mode: tracker.ModeSingle, Level: st.HighestSingle}})
	}
	if st.HighestDouble > 0 {
		out = append(out, ballRef{st.Version, tracker.Chart{Mode: tracker.ModeDouble, Level: st.HighestDouble}})
	}
	return out
}
