# Configuration

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

## Endpoints

Besides the UI:

- `/healthz` is used by the container healthcheck.
- `/api/data.json` is a machine-readable summary of what was loaded:
  - `stats` are the headline stats of the newest version played, which `stats.version` names, and `versions` counts the plays of each version.
  - Per song, each chart of each version has its `version` and its `lineage`: the charts linked to it, itself included, oldest version first, each with its version and chart.
  - Each chart also has its clear and fail counts, and every play with its `version`. `score` and `grade` are `null` for a fail with no result, and `grade` is also `null` when none is known.
  - `best` is the chart's personal best, `null` if no play has a score. It carries across links like the song page's, so its `version` and `chart` can name a linked chart.

## Jacket art

For each song, art is looked up in this order and downloaded once into the data volume:

1. **Custom art directory:** a file named by the song's `image`, or named after the song (`big-daddy.png`).
2. **The song's `image` URL.** For example, a jacket URL copied from piugame.com.
3. **[PIU Scores](https://piuscores.arroweclip.se):** a community site with jackets for about 1,000 songs.
4. **The [PIU Fandom wiki](https://pumpitup.fandom.com):** the lead image of the song's page.
5. **A generated placeholder** (gradient + title), until something is found.

Songs with no art found are looked up again after a week, or straight away when their `image` changes. Network errors are not remembered, so they are simply retried on the next start. Jacket art is © Andamiro, and the app only caches it on your server: it is not part of the image.

## Game art

Grades, plates and level balls are shown with the game's own art, © Andamiro, which is built into the app (see [`COPYRIGHT.txt`](../internal/web/static/piu/COPYRIGHT.txt)). It comes from [PIU Scores](https://piuscores.arroweclip.se):

- A stage break shows its grade cracked, as the result screen does.
- Phoenix charts get Phoenix's level balls. Prime 2 and XX charts, and performance charts in every version, get the balls of XX and older mixes.
- A chart with no ball art, such as S29, gets a drawn ball instead.

## Container image

The image is a static binary on `distroless/static:nonroot` (about 25 MB). It runs with a read-only root filesystem and no capabilities.
