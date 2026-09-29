package art

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"regexp"
	"strings"
)

// The jackets built into the app are PIU Scores' image set, converted to
// WebP; jackets/COPYRIGHT.txt says where they come from.
//
//go:embed jackets/*.webp jackets/index.json
var jacketFiles embed.FS

// Bundle is a set of jackets named like the files on the PIU Scores image
// host: index.json in it maps each name there ("Rockthehouse") to a file.
type Bundle struct {
	fsys  fs.FS
	files map[string]string
	// loose maps names without case or punctuation ("rockthehouse") to the
	// names they come from.
	loose map[string][]string
}

// Jackets returns the jackets built into the app.
func Jackets() (*Bundle, error) {
	sub, err := fs.Sub(jacketFiles, "jackets")
	if err != nil {
		return nil, err
	}
	return LoadBundle(sub)
}

// LoadBundle reads a bundle from fsys, which holds index.json and the files
// it names.
func LoadBundle(fsys fs.FS) (*Bundle, error) {
	b, err := fs.ReadFile(fsys, "index.json")
	if err != nil {
		return nil, err
	}
	bundle := &Bundle{fsys: fsys, loose: map[string][]string{}}
	if err := json.Unmarshal(b, &bundle.files); err != nil {
		return nil, fmt.Errorf("jacket index: %w", err)
	}
	for name := range bundle.files {
		key := strings.ToLower(alnum(name))
		bundle.loose[key] = append(bundle.loose[key], name)
	}
	return bundle, nil
}

// Len is the number of jackets in the bundle.
func (b *Bundle) Len() int { return len(b.files) }

// byURL returns the file of the jacket a PIU Scores image URL names, if the
// bundle has it.
func (b *Bundle) byURL(u string) (string, bool) {
	name, ok := strings.CutPrefix(u, piuScoresSongs)
	if !ok {
		return "", false
	}
	name, ok = strings.CutSuffix(name, ".png")
	if !ok {
		return "", false
	}
	name, err := url.PathUnescape(name)
	if err != nil {
		return "", false
	}
	f, ok := b.files[name]
	return f, ok
}

// featuring is a title's credit of a featured artist, which PIU Scores often
// leaves out of the jacket's name: "Break Through Myself feat. Risa Yuzuki"
// is BreakThroughMyself.png.
var featuring = regexp.MustCompile(`(?i)\s+(?:feat\.?|ft\.)\s.*$`)

// byTitle returns the file of a song's jacket: the one PIU Scores names after
// the title (see PIUScores), or else the only one whose name matches the
// title ignoring case and punctuation. A title with a featured artist that
// matches neither way is tried again without them.
func (b *Bundle) byTitle(title string) (string, bool) {
	for _, name := range piuScoresNames(title) {
		if f, ok := b.files[name]; ok {
			return f, true
		}
	}
	key := strings.ToLower(alnum(title))
	if names := b.loose[key]; key != "" && len(names) == 1 {
		return b.files[names[0]], true
	}
	if short := featuring.ReplaceAllString(title, ""); short != title {
		return b.byTitle(short)
	}
	return "", false
}
