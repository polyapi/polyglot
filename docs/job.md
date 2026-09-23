# Jobs

`polyapi job` is the imperative tree for scheduled jobs and their executions. Create, enable, disable, and run a job without Canopy.

Jobs have **no context** on the platform DTO. Lookup is by ID or name. Function list entries are server functions; pass `id` or `context.name` (or JSON with `functionContext` + `functionName`).

Platform docs: [Jobs](https://docs.polyapi.io/jobs/), [Managing Jobs](https://docs.polyapi.io/jobs/managing.html), [Executions](https://docs.polyapi.io/jobs/executions.html).

## Commands

```bash
polyapi job init --name nightly
polyapi job list
polyapi job get nightly
polyapi job create --name nightly --cron "0 3 * * *" --function billing.weeklyReport
polyapi job update nightly --cron "0 4 * * *"
polyapi job enable nightly
polyapi job disable nightly
polyapi job run nightly
polyapi job delete nightly
polyapi job executions list nightly
polyapi job executions get nightly abc123
polyapi job executions delete nightly abc123
polyapi job executions delete nightly --all
```

`job list` prints NAME, ENABLED, SCHEDULE, and ID. Use `job get` for the function list and full schedule object.

## Create / update fields

| Flag | Notes |
| --- | --- |
| `--name` | Required on create. On update, renames. |
| `--function` | Repeatable. Server function `id`, `context.name`, or JSON `{id,eventPayload,headersPayload,paramsPayload}`. |
| `--functions` | JSON array alternative to `--function`. |
| `--execution-type` | `sequential` or `parallel` (create default `sequential`). |
| `--enabled` | Create default `true`. Prefer `job enable` / `job disable`. |
| `--cron` | Periodical crontab (Zulu). Example: `0 3 * * *`. |
| `--interval` | Interval in minutes. |
| `--on-time` | One-shot ISO 8601 datetime (Zulu). |
| `--schedule` | JSON `{type,value}` (`periodical`, `interval`, or `on_time`). |
| `--delay-minutes` | `acceptableDelayTime` — skip a late run after this many minutes. |

`--cron`, `--interval`, `--on-time`, and `--schedule` are mutually exclusive. A job with no schedule can still be started with `job run`.

Create sends:

```json
{
  "name": "nightly",
  "schedule": { "type": "periodical", "value": "0 3 * * *" },
  "functions": [{ "id": "…" }],
  "executionType": "sequential",
  "enabled": true
}
```

The server function should accept `eventPayload`, `headersPayload`, and `paramsPayload`.

## Run and enable

`job run` `POST`s `/jobs/{id}/trigger` and prints `{jobId, runId, queuedAt}`. It does not change the schedule.

`job enable` / `job disable` `PATCH` `{enabled: true|false}`.

## Executions

`GET /jobs/{id}/executions` with optional `--status`, `--last-hours`, `--last-days`, `--limit`. Status values: `finished`, `job_error`, `with_call_error`, `max_execution_time_reached`, `scheduling_error`.

`executions list` prints ID, STATUS, DURATION, and PROCESSED. `executions get` prints the full execution (including per-function invocation and response).

## Glide

`polyapi job init` writes a JSONC scaffold (default `src/artifacts/jobs/<name>.jsonc`). Jobs have no `--context`. Without `--snippet` it does not call the API. `--snippet` copies a JSON/JSONC snippet as the file (name overwritten).

JSON/JSONC under `**/jobs/**` (or code + `polyConfig`). Author function list entries as `functionContext` + `functionName` (or `id` as `context.name` / UUID). A crontab string is treated as `{type: "periodical", value: …}`.

```json
{
  "name": "nightly",
  "schedule": { "type": "periodical", "value": "0 3 * * *" },
  "functions": [{ "functionContext": "billing", "functionName": "weeklyReport" }],
  "executionType": "sequential"
}
```

The host rewrites those to server-function UUIDs (functions deploy before jobs). Deploy payload is the create DTO fields only (`name`, `schedule`, `functions`, `executionType`, `enabled`). Seed description and other extra keys are not pushed.

See [docs/glide.md](glide.md).
