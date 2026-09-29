# pump-it-up-tracker

A simple read-only web application for personal tracking of Pump It Up scores.

You log each result screen as a few lines of YAML in your Ansible variables. The [`ansible-role-piu-tracker`](https://github.com/spatterIight/ansible-role-piu-tracker) role turns them into a data file. This app shows your songs, per-chart personal bests and progress, and your plays day by day. It also measures you in the game's own terms: your PUMBILITY, and how much of each level you've cleared.

![Home page](docs/screenshots/home.jpg)

| Song page | Activity |
| --- | --- |
| ![Song page](docs/screenshots/song.jpg) | ![Activity page](docs/screenshots/activity.jpg) |
| **Progress** | **A level folder** |
| ![Progress page](docs/screenshots/progress.jpg) | ![A level folder](docs/screenshots/folder.jpg) |

## Quick start

Try it with the sample data, then open <http://localhost:8080>:

```sh
docker run --rm -p 8080:8080 \
  --mount type=bind,src=$PWD/sample/tracker.json,dst=/config/tracker.json,readonly \
  --tmpfs /data \
  ghcr.io/spatteriight/pump-it-up-tracker:latest
```

Or from source (Go 1.24+):

```sh
PIU_TRACKER_DATA_FILE=sample/tracker.json PIU_TRACKER_ART_CACHE_DIR=/tmp/piu-art \
  go run ./cmd/pump-it-up-tracker
```

To run it for real, use the [Ansible role](https://github.com/spatterIight/ansible-role-piu-tracker/blob/main/docs/configuring-piu-tracker.md).

## Logging a play

Each result screen is one entry:

```yaml
piu_tracker_scores:
  - song: Big Daddy
    chart: S11                 # the level ball: S11, D21, SP12, DP20, CoOp2
    date: 2026-09-28           # or "2026-09-28 18:45" to order plays within a day
    score: 938204
    plate: TG
    judgments: {perfect: 506, great: 31, good: 11, bad: 7, miss: 6}
    max_combo: 294
    kcal: 31.137
```

- `song`, `chart` and `date` are required, plus either `score` or both `judgments` and `max_combo`.
- A failed play (the result screen shows `-`) is logged with `broken: true` and no score.
- Plays are from Phoenix unless they say otherwise: add `version: phoenix2`, `version: prime2` or `version: xx` for the others.
- Write `31`, not the cabinet's `031`: YAML reads a leading zero as octal.

Every entry is checked for typos, such as a score that doesn't match its judgments or an unknown key. A deploy with a mistake fails and names the entry. Judgments that don't add up to the chart's note count get a warning. To check a data file yourself:

```sh
go run ./cmd/pump-it-up-tracker validate path/to/tracker.json
```

## Documentation

- [Logging scores](docs/logging-scores.md): every key, failed plays, typo checks and the data file
- [Game versions](docs/game-versions.md): Phoenix 2, Prime 2 and XX plays, and linking a chart across versions
- [Progress](docs/progress.md): PUMBILITY, level folders and the chart list
- [Configuration](docs/configuration.md): environment variables, endpoints, jacket art and game art
- [Development](docs/development.md): tests, adding a game version and releasing

## Copyright

Pump It Up and its art are © Andamiro. The app includes the game's grade, plate and level-ball images and 1,080 song jackets, which come from [PIU Scores](https://piuscores.arroweclip.se); a `COPYRIGHT.txt` beside each set lists them ([game art](internal/web/static/piu/COPYRIGHT.txt), [jackets](internal/art/jackets/COPYRIGHT.txt)). Jackets it downloads for other songs stay on your own server. The list of every Phoenix and Phoenix 2 chart, with how hard players find each, is PIU Scores' too ([`SOURCE.txt`](internal/catalog/data/SOURCE.txt)). This is a personal tracker, not affiliated with Andamiro or PIU Scores.
