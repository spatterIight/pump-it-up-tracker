# pump-it-up-tracker

A simple read-only web application for personal tracking of Pump It Up Game scores.

You log each result screen as a few lines of YAML in your Ansible variables. The [`ansible-role-piu-tracker`](https://github.com/spatterIight/ansible-role-piu-tracker) role renders them into a data file, and this app shows them:

- **Songs:** every song you have played, with jacket art, the level balls of the charts you've played and your best grade.
- **Song pages:** per-chart personal bests and a progress chart (score, misses or perfect %) with grade lines. They also have a table of every attempt, including its judgments, plate, max combo and kcal.
- **Activity:** your plays grouped by day, with new bests marked.

Plays are from Pump It Up Phoenix unless you say otherwise: plays from Prime 2 and XX can be logged alongside them (see [Game versions](#game-versions)).

![Home page](docs/screenshots/home.jpg)

| Song page | Activity |
| --- | --- |
| ![Song page](docs/screenshots/song.jpg) | ![Activity page](docs/screenshots/activity.jpg) |
| **Songs not cleared yet** | **A chart not cleared yet** |
| ![Songs list with songs not cleared yet](docs/screenshots/songs.jpg) | ![Song page with only failed plays](docs/screenshots/failed.jpg) |

## Logging a result screen

This is what one play looks like in the Ansible variables, taken from a real result screen:

```yaml
piu_tracker_scores:
  - song: Big Daddy
    chart: S11                 # the level ball: S11, D21, SP12, DP20, CoOp2
    date: 2026-09-28           # or "2026-09-28 18:45" to order plays within a day
    score: 938204
    plate: TG                  # PG UG EG SG MG TG FG RG, or the full name ("Talented Game")
    judgments: {perfect: 506, great: 31, good: 11, bad: 7, miss: 6}
    max_combo: 294
    kcal: 31.137
    note: "finally"            # optional
```

Only `song`, `chart` and `date` are always required. Everything else is optional, as long as there is either a `score` or both `judgments` and `max_combo` to work it out from. Add `broken: true` for a stage break. A play from another version of the game than Phoenix names it with `version` (see [Game versions](#game-versions)).

### Failed plays

When you fail a chart and the result screen shows `-` instead of a score, log the play with `broken: true` and no result:

```yaml
  - {song: DUEL, chart: S13, date: "2026-09-08", broken: true}
```

Such a play may only have `song`, `chart`, `date`, `kcal` and `note` (and `version`). This works the same way in every version. It has no score or grade, and never counts as a clear or a personal best. It shows as "Failed" in the attempts table and the activity list, and as a marker along the bottom of the progress chart. A song or chart with no clear yet shows "Not cleared" with its number of attempts instead of a grade.

A stage break that still shows a score is logged with `broken: true` and its score (or judgments and max combo).

### Typo checks

Every entry is checked before it is shown. A deploy with a mistake in it fails and names the entry:

- **Score against judgments.** When the judgments and max combo are both given, the score must match what the game would award for them:

  `1,000,000 × (0.995 × (Perfect + 0.6·Great + 0.2·Good + 0.1·Bad) + 0.005 × MaxCombo) / Notes`
- **Grade against score.** A `grade`, if given, must match the Phoenix grade table. Grades are worked out from the score otherwise.
- **Failed plays.** A `broken: true` play with no score cannot have a `grade`, `plate`, `judgments` or `max_combo`. Without `broken: true`, a play with no score is rejected.
- **Everything else:** chart notation, dates, plate names, and unknown keys (such as `perfects:`) are rejected with an explanation.

These are Phoenix's checks. Plays from other versions are checked by their own version's rules, and a problem with such a rule names the version ("plate is not used in Prime 2"); see [Game versions](#game-versions).

> [!IMPORTANT]
> The cabinet pads numbers with zeros (`031`), and YAML reads numbers with a leading zero as octal, so `great: 031` arrives as 25. Write `great: 31`. The score check catches this when judgments are given.

### Data file

The role writes the variables to a JSON file that this app reads (`schema_version: 1`):

```json
{
  "schema_version": 1,
  "player": { "name": "PUMP IT UP" },
  "songs": { "Conflict": { "artist": "Siromaru + Cranky", "bpm": "160", "image": "" } },
  "scores": [
    { "song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": 938204 },
    { "song": "DUEL", "chart": "S13", "date": "2026-09-08", "broken": true },
    { "song": "Vook", "chart": "S7", "date": "2025-07-25 22:03", "version": "prime2", "score": 419400, "grade": "A" }
  ]
}
```

`songs` is optional metadata keyed by title. It holds `artist`, `bpm`, `image`, which is either an image URL or a file name in the custom art directory, and `lineages` (see [Chart continuity](#chart-continuity)). Titles are matched case-insensitively. See [`sample/tracker.json`](sample/tracker.json) for a complete example.

## Game versions

Each play belongs to a version of Pump It Up, named by its `version` key. A play without one is from Phoenix, so data files from before versions existed mean what they always did, and `schema_version` is still 1.

```yaml
  - song: Le Grand Bleu
    chart: S7
    version: prime2
    date: "2025-09-09 18:15"
    score: 1038500
    grade: S
    judgments: {perfect: 496, great: 31, good: 2, bad: 1, miss: 0}
    max_combo: 460
    kcal: 19.18
```

| `version` | Version | Grades | Checked |
| --- | --- | --- | --- |
| `phoenix` (default) | Pump It Up Phoenix | SSS+ to F | Everything in [Typo checks](#typo-checks) |
| `prime2` | Pump It Up Prime 2 | SS, S, A, B, C, D, F | A `score` is given, is a multiple of 100 and is in the range its judgments allow; a `grade` is one of the version's; no `plate` |
| `xx` | Pump It Up XX (20th Anniversary Edition) | SSS, SS, S, A, B, C, D, F | The same as Prime 2 |

The version can also be written with capitals or spaces (`Prime 2`). Any other value is rejected, and the problem lists the supported ones.

**The cabinet is the record.** A play's score and grade are kept and shown exactly as the result screen showed them, in its own version's scale: nothing is converted into another version's. Scores of different versions only look alike. A Prime 2 score can be over 1,000,000 (Le Grand Bleu S7's 1,038,500 above), and Prime 2's grades are not Phoenix's.

**Prime 2 and XX** plays take `score`, `grade`, `judgments`, `max_combo`, `kcal`, `note` and `broken` just as Phoenix plays do, with these differences:

- `score` is required: it cannot be worked out from the judgments (see below). There is no fixed upper limit. Both versions round scores down to a multiple of 100, so any other score is rejected as a typo, and so is a score outside the range its judgments allow (see below).
- `plate` is rejected, since neither version has plates.
- `grade` is optional and shown as logged, as long as the version has it: `SSS` is not a Prime 2 grade (XX renamed Prime 2's SS, gold S and silver S to SSS, SS and S). A play without one shows no grade. Log a gold or silver Prime 2 S as `S`.
- A cracked grade on an XX result screen means the life bar ran out: log it with `broken: true`, like any stage break with a score.

**Score against judgments.** Pump It Up scored plays the same way from Zero to XX ([NamuWiki](https://namu.wiki/w/%ED%8E%8C%ED%94%84%20%EC%9E%87%20%EC%97%85/%EA%B8%B0%EB%B3%B8%20%EC%8B%9C%EC%8A%A4%ED%85%9C)):

- Each judgment scores points: Perfect 1,000, Great 500, Good 100, Bad −200, and Miss −500, or −300 on a hold.
- Each Perfect or Great from the 51st combo on scores 1,000 more. Goods keep the combo without adding to it; Bads and Misses break it.
- A row of three notes scores 1.5 times as much, and a row of four twice as much.
- An S or better adds a grade bonus: 100,000 for an S, and up to 300,000 for the best grade.
- Above level 10, the total is multiplied by the level ÷ 10, and it is multiplied by 1.2 for doubles and by 1.2 in rank mode. It is then rounded down to a multiple of 100.

The exact score depends on where the combo broke, which misses were on holds and how many notes each row had, and the result screen shows none of that. So a Prime 2 or XX score is checked against the range its judgments allow instead:

- **The lowest** assumes the combo broke where it costs the most bonus, single notes, every Miss off a hold, no multiplier, and the S bonus only when an S or better is logged. It uses the max combo when given.
- **The highest** assumes rows of four notes, every Perfect and Great past the 50th of one combo, every Miss on a hold, the best grade bonus (none below an S) and every multiplier. Co-op scores have no highest, since their scoring is not documented.

All 15 Prime 2 result screens fall within their range, and 6 of them exactly on the lowest. The lowest catches most typos, such as a misread digit (1,036,500 for 1,038,500) or a missing S bonus. The highest is loose, and only catches a digit too many. A stage break's result screen does not add up the same way, so only its rounding is checked.

What is not checked yet, and why:

- **Grade table.** The documented rule gives an S for no Miss and at least 95% of a weighted accuracy, then A, B, C and D at 90, 85, 80 and 75%. It gives an A for a real Prime 2 B (Yog-Sothoth S9, 61 misses) and a B for a real XX C. The accuracy also depends on the number of hold judgments, which the result screen does not show. So grades are not checked for these versions.

The grade check will be turned on once a rule reproduces real result screens. Prime 2 and XX score plays the same way but name grades differently, so each has a scoring system of its own.

**Charts** of different versions are different charts, even when the song and level are the same: levels get re-rated between versions, and steps sometimes change. A Prime 2 S7 and a Phoenix S7 of the same song are listed separately, each with its own personal best and history, unless you link them (see [Chart continuity](#chart-continuity)).

**Personal bests** are only ever compared within a scoring system: between plays of the same chart in versions that score the same way. A version's first play of a chart is its first clear there, whatever you scored on it in another version.

**The headline stats** on the home page cover the newest version you have played, Phoenix today: plays, songs, charts, hardest clears, fails, new bests and the grade breakdown. Under them, a line counts your plays per version. When you log your first play from a newer version, the headline stats move to it by themselves.

**On the pages:**

- **Songs list:** each song shows the newest version it was played in: its level balls and best grade. A song whose newest version is older than the newest one you played has a version badge. The version filter above the list (default: all versions) lists only the songs played in one version, and shows each in that version.
- **Song page:** the charts are grouped by version under a header naming it, newest version first. There are no headers when every chart is from the newest version you played.
- **Attempts and activity:** plays from a version older than the newest one you played have a version badge, and each play's grade is from its own version. The activity page has the same version filter.
- **Jacket art** is looked up by song title, so every version of a song shares it.

## Chart continuity

When the same step chart is in two versions, possibly at a different level, link them in the song's metadata with a lineage: a mapping from each version to the chart as that version labels it.

```yaml
piu_tracker_songs:
  Katkoi:
    lineages:
      - {phoenix: S7, next: S8}   # Phoenix's S7 is the same steps as S8 in a later version
      - {prime2: D17, phoenix: D18}
```

A lineage is declared once for all the plays of its charts, before or after, and can span any number of versions, in any order: they are put in release order. A song can have several. A lineage is rejected when:

- it names fewer than two versions, or the same version twice (`prime2` and `Prime 2`);
- a version or chart is not recognised;
- one of its charts has no plays, for example because of a typo in the version or chart, or because it is another song's: a lineage stays within one song;
- one of its charts is already in another of the song's lineages.

The song page shows a lineage as one chart, listed under its newest version and labelled with each version's level, such as "S7 (Phoenix) → S8 (Next)":

- **Attempts** span the whole lineage. Each play names its chart, and has a version badge when it is from a version older than the newest one you played.
- **Personal bests, first clear and best plate** carry across a link only when both versions use the same scoring system. Linking a Phoenix chart to the same chart in a future version that keeps Phoenix scoring continues its history: the next version's first play is compared with the best from Phoenix. The linked charts then share one history, in date order, so a best set in the newer version also counts for the older chart if you play it there again. Across a link between different scoring systems, such as Prime 2 → Phoenix, each version keeps its own. The page shows the newest version's personal best, with the older versions' bests beside it.
- **Progress charts:** misses and perfect % do not depend on the scoring system, so they span the whole lineage, with a dashed line where it moves to another chart. The score chart spans the lineage only while the scoring system stays the same. Across different ones it is drawn in separate parts, one per scoring system, each headed with its versions, and never as one line.

![A Prime 2 chart continued in Phoenix: one chart, with the Prime 2 best beside the Phoenix one and the score chart in two parts](docs/screenshots/lineage.jpg)

## Adding a game version

Versions are listed in release order in [`internal/tracker/version.go`](internal/tracker/version.go). Each has an ID for the data file, a display name and a scoring system. Versions that share a scoring system share personal bests across chart links.

- **If the new version keeps the scoring of the one before (such as Phoenix):** add a version entry after it, pointing at the same scoring system (`PhoenixScoring`). No scoring code is needed. Add tests to [`internal/tracker/versions_test.go`](internal/tracker/versions_test.go) that load real result screens of the version (as `testdata/result-screens.json` does for Prime 2 and XX).
- **If it changes the scoring:** also write a new scoring system, which implements the `ScoringSystem` interface in [`internal/tracker/scoring.go`](internal/tracker/scoring.go): `Name`, `ComputesScores`, `ComputeScore`, `CheckScore`, `Grade`, `Grades`, `GradeThresholds`, `MaxScore` and `HasPlates`. Make it a pointer (a version list whose scoring system cannot be compared with `==` is refused). Only work out or check a score formula or grade table that reproduces every real result screen you have. Until one does, return `false` from `ComputesScores` and from `Grade`, and the version's scores and grades are taken as logged; a range, as for Prime 2 and XX, can still be checked in `CheckScore`. Add result screens to the tests, and describe what is checked in [Game versions](#game-versions).

Once the version is added, a play logs it with `version`, and song lineages link its charts to Phoenix's. When you log a play from it, it becomes the newest version played and the headline stats move to it. Adding a version is a minor release.

## Jacket art

For each song, art is looked up in this order and downloaded once into the data volume:

1. **Custom art directory:** a file named by the song's `image`, or named after the song (`big-daddy.png`).
2. **The song's `image` URL.** For example, a jacket URL copied from piugame.com.
3. **[PIU Scores](https://piuscores.arroweclip.se):** a community site with jackets for about 1,000 songs.
4. **The [PIU Fandom wiki](https://pumpitup.fandom.com):** the lead image of the song's page.
5. **A generated placeholder** (gradient + title), until something is found.

Songs with no art found are looked up again after a week, or straight away when their `image` changes. Network errors are not remembered, so they are simply retried on the next start. Jacket art is © Andamiro: the app only caches it on your server and never bakes it into the image.

## Configuration

The container is configured through environment variables:

| Variable | Default | |
| --- | --- | --- |
| `PIU_TRACKER_DATA_FILE` | `/config/tracker.json` | The score data |
| `PIU_TRACKER_LISTEN_ADDRESS` | `:8080` | |
| `PIU_TRACKER_BASE_PATH` | `/` | Serve under a path prefix, e.g. `/piu` (without stripping it in the proxy) |
| `PIU_TRACKER_ART_CACHE_DIR` | `/data/art` | Must be writable when fetching is enabled |
| `PIU_TRACKER_ART_CUSTOM_DIR` | `/art` | Optional, your own images |
| `PIU_TRACKER_ART_FETCH_ENABLED` | `true` | `false` keeps the app entirely offline |
| `PIU_TRACKER_ART_SOURCES` | `piuscores,fandom` | Order of the online sources |
| `PIU_TRACKER_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `TZ` | UTC | Time zone for "today" and "yesterday" |

Endpoints besides the UI:
- `/healthz` is used by the container healthcheck.
- `/api/data.json` is a machine-readable summary of what was loaded:
  - `stats` are the headline stats of the newest version played, which `stats.version` names, and `versions` counts the plays of each version.
  - Per song, each chart of each version has its `version` and its `lineage`: the charts linked to it, itself included, oldest version first, each with its version and chart.
  - Each chart also has its clear and fail counts, and every play with its `version`. `score` and `grade` are `null` for a fail with no result, and `grade` is also `null` when none is known.
  - `best` is the chart's personal best, `null` if no play has a score. It carries across links like the song page's, so its `version` and `chart` can name a linked chart.

## Running it

```sh
# From source (Go 1.24+)
PIU_TRACKER_DATA_FILE=sample/tracker.json PIU_TRACKER_ART_CACHE_DIR=/tmp/piu-art \
  go run ./cmd/pump-it-up-tracker

# Container
docker run --rm -p 8080:8080 \
  --mount type=bind,src=$PWD/sample/tracker.json,dst=/config/tracker.json,readonly \
  --tmpfs /data \
  ghcr.io/spatteriight/pump-it-up-tracker:latest

# Check a data file without starting the server
go run ./cmd/pump-it-up-tracker validate path/to/tracker.json
# path/to/tracker.json is valid: 54 plays of 28 songs (38 Phoenix, 1 XX, 15 Prime 2), 8 of them failed
```

The image is a static binary on `distroless/static:nonroot` (about 20 MB). It runs with a read-only root filesystem and no capabilities.

## Development

```sh
go vet ./... && go test ./...
```

It uses the standard library only. The UI is server-rendered HTML templates with SVG charts drawn in Go and a little JavaScript for filters, tabs and tooltips; every page also works without JavaScript. The pages make no external requests: the Chakra Petch font (SIL OFL, see [`internal/web/static/fonts/OFL-ChakraPetch.txt`](internal/web/static/fonts/OFL-ChakraPetch.txt)) and all art are served locally.

## Releasing

Push a tag such as `v0.1.0`. The release workflow publishes `ghcr.io/spatteriight/pump-it-up-tracker:0.1.0` (plus `0.1` and `latest`) for amd64 and arm64. A release that adds a game version or a data file key is a minor one.
