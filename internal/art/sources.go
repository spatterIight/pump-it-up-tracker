package art

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

// PIUScores proposes images from the PIU Scores community site
// (https://piuscores.arroweclip.se), which names each jacket after the song
// title with everything but ASCII letters and digits removed:
// "Rock the house" is songs/Rockthehouse.png.
type PIUScores struct {
	// BaseURL defaults to https://piuimages.arroweclip.se/songs/.
	BaseURL string
}

func (PIUScores) Name() string { return "piuscores" }

func (p PIUScores) URLs(_ context.Context, s Song) ([]string, error) {
	base := p.BaseURL
	if base == "" {
		base = "https://piuimages.arroweclip.se/songs/"
	}
	var out []string
	seen := map[string]bool{}
	// The title as written first, then with every word capitalised, since
	// the file name keeps the site's own capitalisation of the title.
	for _, name := range []string{alnum(s.Title), alnum(capitalizeWords(s.Title))} {
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, base+url.PathEscape(name)+".png")
		}
	}
	return out, nil
}

func alnum(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func capitalizeWords(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// Fandom proposes the lead image of the song's page on the PIU Fandom wiki
// (https://pumpitup.fandom.com), found through the MediaWiki API.
type Fandom struct {
	// APIURL defaults to https://pumpitup.fandom.com/api.php.
	APIURL    string
	Client    *http.Client
	UserAgent string
}

func (Fandom) Name() string { return "fandom" }

func (f Fandom) URLs(ctx context.Context, s Song) ([]string, error) {
	api := f.APIURL
	if api == "" {
		api = "https://pumpitup.fandom.com/api.php"
	}
	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}
	q := url.Values{
		"action":        {"query"},
		"format":        {"json"},
		"formatversion": {"2"},
		"prop":          {"pageimages"},
		"piprop":        {"original"},
		"redirects":     {"1"},
		"titles":        {s.Title},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wiki API: %s", resp.Status)
	}
	var body struct {
		Query struct {
			Pages []struct {
				Missing  bool `json:"missing"`
				Original struct {
					Source string `json:"source"`
				} `json:"original"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("wiki API: %w", err)
	}
	var out []string
	for _, p := range body.Query.Pages {
		if !p.Missing && isURL(p.Original.Source) {
			out = append(out, p.Original.Source)
		}
	}
	return out, nil
}
