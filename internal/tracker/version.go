package tracker

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// Version is a release of Pump It Up that plays are logged from.
type Version struct {
	// ID is how the version is written in the data file ("prime2").
	ID string
	// Name is how the version is shown ("Prime 2").
	Name string
	// Scoring is how the version scores and grades plays. Personal bests
	// carry across a chart link only between versions whose scoring systems
	// put scores on the same scale.
	Scoring ScoringSystem
	// Pumbility prices charts for the version's PUMBILITY, nil when the
	// version has none.
	Pumbility PumbilityFormula

	// order is the version's place in release order, counted from the oldest.
	order int
}

// versions lists the game versions this build reads, in release order. See
// docs/development.md, "Adding a game version".
var versions = []Version{
	{ID: "prime2", Name: "Prime 2", Scoring: Prime2Scoring},
	{ID: "xx", Name: "XX", Scoring: XXScoring},
	{ID: "phoenix", Name: "Phoenix", Scoring: PhoenixScoring, Pumbility: PhoenixPumbility},
	{ID: "phoenix2", Name: "Phoenix 2", Scoring: Phoenix2Scoring, Pumbility: Phoenix2Pumbility},
}

// DefaultVersion is the ID of the version of a play that does not name one.
const DefaultVersion = "phoenix"

// Versions returns the game versions this build reads, in release order.
func Versions() []Version { return slices.Clone(versions) }

// Order is the version's place in release order, counted from 0 for the
// oldest.
func (v *Version) Order() int { return v.order }

// Before reports whether v was released before o.
func (v *Version) Before(o *Version) bool { return v.order < o.order }

// IsDefault reports whether v is the version of plays that do not name one.
func (v *Version) IsDefault() bool { return v.ID == DefaultVersion }

// versionSet is the game versions a data file is read with.
type versionSet []*Version

// newVersionSet numbers the versions in release order.
func newVersionSet(vs []Version) (versionSet, error) {
	set := make(versionSet, len(vs))
	seen := map[string]bool{}
	for i, v := range vs {
		key := normalizeVersion(v.ID)
		if v.ID == "" || key != v.ID || v.Name == "" || v.Scoring == nil {
			return nil, fmt.Errorf("game version %d (%q) needs a lowercase ID of letters and digits, a name and a scoring system", i, v.ID)
		}
		if seen[key] {
			return nil, fmt.Errorf("game version %q is listed twice", v.ID)
		}
		// Scoring systems are compared with ==, which panics on a value
		// holding a slice, map or function.
		if !reflect.TypeOf(v.Scoring).Comparable() {
			return nil, fmt.Errorf("game version %q: its scoring system %s cannot be compared with ==; make it a pointer", v.ID, v.Scoring.Name())
		}
		if v.Scoring.Scale() == nil {
			return nil, fmt.Errorf("game version %q: its scoring system %s has no score scale", v.ID, v.Scoring.Name())
		}
		seen[key] = true
		v.order = i
		set[i] = &v
	}
	if !seen[DefaultVersion] {
		return nil, fmt.Errorf("the default game version %q is missing", DefaultVersion)
	}
	return set, nil
}

// normalizeVersion lowercases a version as written and drops spaces, dashes
// and underscores, so that "Prime 2" and "PRIME2" both read as prime2.
func normalizeVersion(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '_', '\t':
			return -1
		}
		return r
	}, strings.ToLower(strings.TrimSpace(s)))
}

// lookup finds a version by ID, as written in a data file.
func (vs versionSet) lookup(s string) (*Version, bool) {
	key := normalizeVersion(s)
	for _, v := range vs {
		if v.ID == key {
			return v, true
		}
	}
	return nil, false
}

// list names every version, for a problem report: "phoenix, prime2 or xx".
func (vs versionSet) list() string {
	ids := make([]string, len(vs))
	for i, v := range vs {
		ids[i] = v.ID
	}
	slices.Sort(ids)
	if len(ids) == 1 {
		return ids[0]
	}
	return strings.Join(ids[:len(ids)-1], ", ") + " or " + ids[len(ids)-1]
}
