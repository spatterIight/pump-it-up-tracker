# Development

```sh
go vet ./... && go test ./...
```

It uses the standard library only. The UI is server-rendered HTML templates with SVG charts drawn in Go and a little JavaScript for filters, tabs and tooltips, and for swapping pages in without a full load (so the screen never blanks between them); every page also works without JavaScript. The pages make no external requests: the Chakra Petch font (SIL OFL, see [`internal/web/static/fonts/OFL-ChakraPetch.txt`](../internal/web/static/fonts/OFL-ChakraPetch.txt)) and all art are served locally. The game's grade, plate and level-ball images in [`internal/web/static/piu`](../internal/web/static/piu) are © Andamiro, not the project's own work: [`COPYRIGHT.txt`](../internal/web/static/piu/COPYRIGHT.txt) there says where they come from. The same goes for the jackets in [`internal/art/jackets`](../internal/art/jackets) and [their `COPYRIGHT.txt`](../internal/art/jackets/COPYRIGHT.txt). Keep both up to date when adding or replacing any. The chart list in [`internal/catalog/data`](../internal/catalog/data) is PIU Scores' too, and [`SOURCE.txt`](../internal/catalog/data/SOURCE.txt) there says when it was exported.

## Refreshing the chart list

The [Progress](progress.md) page, the chart facts on song pages, the links between Phoenix and Phoenix 2 charts and the note-count check all come from the chart list: one CSV file per version in `internal/catalog/data`, named after the version's ID. Each is PIU Scores' chart export for the version, as it comes. To refresh one, download it again with the URL in `SOURCE.txt` (the `Mix` parameter names the version as PIU Scores does, such as `Phoenix2`), update the date there, and run the tests: they check that the list reads and has the charts they expect. Refreshing it is a patch release.

## Refreshing the jackets

To refresh the jackets from a new download of PIU Scores' `songs/` images, convert each PNG to WebP at quality 85 (keeping transparency where a jacket has any), and regenerate `index.json`, which maps each file name on the host (without `.png`) to its file. The tests check that every file is indexed and is a WebP image.

## Adding a game version

Versions are listed in release order in [`internal/tracker/version.go`](../internal/tracker/version.go). Each has an ID for the data file, a display name, a scoring system and, from Phoenix on, a PUMBILITY formula. A scoring system puts scores on a scale (`ScoreScale`): versions whose scoring systems share a scale share personal bests across chart links, even if they grade differently, as Phoenix and Phoenix 2 do.

- **If the new version keeps the scoring and grades of the one before:** add a version entry after it, pointing at the same scoring system. No scoring code is needed. Add tests to [`internal/tracker/versions_test.go`](../internal/tracker/versions_test.go) that load real result screens of the version (as `testdata/result-screens.json` does for Prime 2 and XX).
- **If it keeps the score formula but changes the grades (as Phoenix 2 did):** add a `phoenixScoring` with the new grade cutoffs in [`internal/tracker/scoring.go`](../internal/tracker/scoring.go). It keeps Phoenix's scale, so personal bests carry over from Phoenix. List only the cutoffs you can check; a score below the last one is graded as logged.
- **If it changes the scoring:** also write a new scoring system, which implements the `ScoringSystem` interface in [`internal/tracker/scoring.go`](../internal/tracker/scoring.go): `Name`, `Scale`, `ComputesScores`, `ComputeScore`, `CheckScore`, `Grade`, `Grades`, `GradeThresholds`, `MaxScore` and `HasPlates`. Make it a pointer (a version list whose scoring system cannot be compared with `==` is refused), with a scale of its own. Only work out or check a score formula or grade table that reproduces every real result screen you have. Until one does, return `false` from `ComputesScores` and from `Grade`, and the version's scores and grades are taken as logged, as for Prime 2 and XX. Add result screens to the tests, and describe what is checked in [Game versions](game-versions.md).
- **PUMBILITY:** give the version the formula it prices charts with (`PumbilityFormula`, in [`internal/tracker/pumbility.go`](../internal/tracker/pumbility.go)), tested against real values from the game's breakdown, or none until one is known.
- **Chart list:** add PIU Scores' chart export for the version as `internal/catalog/data/<id>.csv` (see [Refreshing the chart list](#refreshing-the-chart-list)). Its charts are then linked to the previous versions' by chart ID, and the Progress page covers the version.

Every grade the version awards needs its art in `internal/web/static/piu/letters`, plain and cracked, which the tests check. If the version has level balls of its own, put them in a folder named after the version without spaces (as `difficulty/Phoenix2`), trimmed of transparent margins like the others; otherwise its charts get the balls of XX and older mixes.

Once the version is added, a play logs it with `version`, and song lineages link its charts to Phoenix's. When you log a play from it, it becomes the newest version played and the headline stats move to it. Adding a version is a minor release.

## Releasing

Push a tag such as `v0.1.0`. The release workflow publishes `ghcr.io/spatteriight/pump-it-up-tracker:0.1.0` (plus `0.1` and `latest`) for amd64 and arm64. A release that adds a game version or a data file key is a minor one.
