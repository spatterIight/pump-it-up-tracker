# pump-it-up-tracker

A simple read-only web application for personal tracking of Pump It Up Game scores.

You log each result screen as a few lines of YAML in your Ansible variables. The [`ansible-role-piu-tracker`](https://github.com/spatterIight/ansible-role-piu-tracker) role renders them into a data file, and this app shows them:

- **Songs:** every song you have played, with jacket art, the level balls of the charts you've played and your best grade.
- **Song pages:** per-chart personal bests and a progress chart (score, misses or perfect %) with grade lines. They also have a table of every attempt, including its judgments, plate, max combo and kcal.
- **Activity:** your plays grouped by day, with new bests marked.

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

Only `song`, `chart` and `date` are always required. Everything else is optional, as long as there is either a `score` or both `judgments` and `max_combo` to work it out from. Add `broken: true` for a stage break.

### Failed plays

When you fail a chart and the result screen shows `-` instead of a score, log the play with `broken: true` and no result:

```yaml
  - {song: DUEL, chart: S13, date: "2026-09-08", broken: true}
```

Such a play may only have `song`, `chart`, `date`, `kcal` and `note`. It has no score or grade, and never counts as a clear or a personal best. It shows as "Failed" in the attempts table and the activity list, and as a marker along the bottom of the progress chart. A song or chart with no clear yet shows "Not cleared" with its number of attempts instead of a grade.

A stage break that still shows a score is logged with `broken: true` and its score (or judgments and max combo).

### Typo checks

Every entry is checked before it is shown. A deploy with a mistake in it fails and names the entry:

- **Score against judgments.** When the judgments and max combo are both given, the score must match what the game would award for them:

  `1,000,000 × (0.995 × (Perfect + 0.6·Great + 0.2·Good + 0.1·Bad) + 0.005 × MaxCombo) / Notes`
- **Grade against score.** A `grade`, if given, must match the Phoenix grade table. Grades are worked out from the score otherwise.
- **Failed plays.** A `broken: true` play with no score cannot have a `grade`, `plate`, `judgments` or `max_combo`. Without `broken: true`, a play with no score is rejected.
- **Everything else:** chart notation, dates, plate names, and unknown keys (such as `perfects:`) are rejected with an explanation.

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
    { "song": "DUEL", "chart": "S13", "date": "2026-09-08", "broken": true }
  ]
}
```

`songs` is optional metadata keyed by title. It holds `artist`, `bpm`, and `image`, which is either an image URL or a file name in the custom art directory. Titles are matched case-insensitively. See [`sample/tracker.json`](sample/tracker.json) for a complete example.

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
- `/api/data.json` is a machine-readable summary of what was loaded: per chart, the best play (`null` if none has a score), clear and fail counts, and every play, with `score` and `grade` `null` for a fail with no result.

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
# path/to/tracker.json is valid: 36 plays of 15 songs, 7 of them failed
```

The image is a static binary on `distroless/static:nonroot` (about 20 MB). It runs with a read-only root filesystem and no capabilities.

## Development

```sh
go vet ./... && go test ./...
```

It uses the standard library only. The UI is server-rendered HTML templates with SVG charts drawn in Go and a little JavaScript for filters, tabs and tooltips; every page also works without JavaScript. The pages make no external requests: the Chakra Petch font (SIL OFL, see [`internal/web/static/fonts/OFL-ChakraPetch.txt`](internal/web/static/fonts/OFL-ChakraPetch.txt)) and all art are served locally.

## Releasing

Push a tag such as `v0.1.0`. The release workflow publishes `ghcr.io/spatteriight/pump-it-up-tracker:0.1.0` (plus `0.1` and `latest`) for amd64 and arm64.
