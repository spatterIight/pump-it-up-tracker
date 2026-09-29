# Development

```sh
go vet ./... && go test ./...
```

It uses the standard library only. The UI is server-rendered HTML templates with SVG charts drawn in Go and a little JavaScript for filters, tabs and tooltips; every page also works without JavaScript. The pages make no external requests: the Chakra Petch font (SIL OFL, see [`internal/web/static/fonts/OFL-ChakraPetch.txt`](../internal/web/static/fonts/OFL-ChakraPetch.txt)) and all art are served locally.

## Adding a game version

Versions are listed in release order in [`internal/tracker/version.go`](../internal/tracker/version.go). Each has an ID for the data file, a display name and a scoring system. Versions that share a scoring system share personal bests across chart links.

- **If the new version keeps the scoring of the one before (such as Phoenix):** add a version entry after it, pointing at the same scoring system (`PhoenixScoring`). No scoring code is needed. Add tests to [`internal/tracker/versions_test.go`](../internal/tracker/versions_test.go) that load real result screens of the version (as `testdata/result-screens.json` does for Prime 2 and XX).
- **If it changes the scoring:** also write a new scoring system, which implements the `ScoringSystem` interface in [`internal/tracker/scoring.go`](../internal/tracker/scoring.go): `Name`, `ComputesScores`, `ComputeScore`, `CheckScore`, `Grade`, `Grades`, `GradeThresholds`, `MaxScore` and `HasPlates`. Make it a pointer (a version list whose scoring system cannot be compared with `==` is refused). Only work out or check a score formula or grade table that reproduces every real result screen you have. Until one does, return `false` from `ComputesScores` and from `Grade`, and the version's scores and grades are taken as logged, as for Prime 2 and XX. Add result screens to the tests, and describe what is checked in [Game versions](game-versions.md).

Once the version is added, a play logs it with `version`, and song lineages link its charts to Phoenix's. When you log a play from it, it becomes the newest version played and the headline stats move to it. Adding a version is a minor release.

## Releasing

Push a tag such as `v0.1.0`. The release workflow publishes `ghcr.io/spatteriight/pump-it-up-tracker:0.1.0` (plus `0.1` and `latest`) for amd64 and arm64. A release that adds a game version or a data file key is a minor one.
