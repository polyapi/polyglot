# Code audit

Read-only review of recent `trigger init` work and the trigger/Glide path that consumes the generated files. No patches; direction only.

The init command itself matches the request. The gap is that error-handler is now a first-class local artifact, while create, Glide compare, and remote matching still treat triggers as webhook-shaped.

## 1. Summary

`trigger init --type webhook|error-handler` does what was asked: required `--type`, matching JSONC, tests, and docs. The generated error-handler files are not fully carried through the rest of the trigger pipeline. Glide’s source-change check and remote lookup still assume a webhook handle, so an error-handler file can deploy as a no-op PATCH or attach to the wrong remote. `trigger create --error-handler-path` still always sends `waitForResponse: true`, which contradicts the new init/docs.

Working tree is the whole product vs `origin/main` (no committed range). This review is the trigger-init change plus the trigger/Glide code that consumes those files.

## 2. Suggestions

### High

**What:** Glide does not treat webhook vs error-handler as a source change.

**Where:** `src/glide/load.go` — `triggerSourceDestChanged`

**Why:** Each field is compared only when **both** local and remote have it. A local error-handler file (`source.errorHandler.path`) against a remote webhook trigger (`webhookHandleId`) returns false. `applyOne` in `src/glide/sync.go` then PATCHes only `name` / `waitForResponse` / `enabled` (`outboundUpdate`). `polyapi trigger init --type error-handler` followed by `deploy push` on an existing same-name trigger can report updated while the source stays a webhook.

**Direction:** Fail when one side has `webhookHandleId` and the other has `errorHandler`, or when one side has a field the other lacks. Add a Glide test that inits/loads an error-handler JSONC against a webhook remote and expects the existing “delete and recreate” error.

**What:** `trigger create` always sends `waitForResponse: true`, including for error-handler sources.

**Where:** `src/cli/trigger.go` — `runTriggerCreate`

**Why:** Init and `docs/trigger.md` omit `waitForResponse` for error-handler (platform stores false). Create still does:

```go
payload := map[string]any{
    "waitForResponse": boolFlagOr(cmd, "wait-for-response", true),
    ...
}
```

There is no test for `--error-handler-path` (`tests/cli/trigger_test.go` only covers webhook create). Users who create via CLI vs init+deploy send different bodies.

**Direction:** If `--error-handler-path` is set and `--wait-for-response` was not passed, omit the field (same as the JSONC). If the user passed the flag, send it. Add `TestTriggerCreateErrorHandler` that asserts nested `source.errorHandler.path` and no `waitForResponse`.

### Medium

**What:** `--snippet` ignores `--type` but `--type` is still required.

**Where:** `src/cli/resource_init.go` — `runResourceInit` (snippet branch vs `NeedsTriggerType` branch)

**Why:** `bodyFromSnippet` only overlays `name` / `polyType`. A user can run `--type webhook --snippet <error-handler snippet>` and get error-handler JSONC with no warning. That undercuts the new flag.

**Direction:** Either make `--type` optional when `--snippet` is set, or after overlay require `source.webhookHandleId` vs `source.errorHandler` to match `--type` and fail with usage if they disagree.

**What:** Remote matching for unnamed or name-colliding error-handler triggers ignores `errorHandler.path`.

**Where:** `src/glide/sync.go` — `findRemoteTrigger`

**Why:** Name match is first (fine for the scaffold). Fallback is `webhookHandleId` + `serverFunctionId`. Two error-handler triggers to the same function both have empty `webhookHandleId` and can match the first remote with that destination.

**Direction:** If `errorHandler.path` is set, match on path + destination. Treat empty `webhookHandleId` as “not a webhook match,” not as equal to another empty handle.

**What:** No Glide round-trip test for the new error-handler JSONC.

**Where:** `tests/glide/glide_test.go` — `TestWebhookAndTriggerGlideRewriteRefs` (webhook only); `tests/glide/scaffold_test.go` — `TestScaffoldTriggerJSONCKinds` (JSON shape only)

**Why:** Init now writes `source.errorHandler` + `destination.serverFunctionId` and no `waitForResponse`. Create, validate refs (`src/glide/validate.go` trigger case), and rewrite (`rewriteRefs`) never look at `errorHandler.path`. Placeholders like `example.handler` will be looked up as a **server function** on destination (intended) and left as a raw path on source (probably intended). That path is untested.

**Direction:** Push an error-handler scaffold with a real destination function through `Engine.Push` and assert the posted body keeps `errorHandler.path` and omits `waitForResponse`.

**What:** Two mutually exclusive booleans both register `--type`.

**Where:** `src/cli/resource_init.go` — `resourceInitKind.NeedsFuncType` / `NeedsTriggerType`; `newResourceInitCommand`; `runResourceInit`

**Why:** Setting both would register `--type` twice. Help, examples, and scaffold selection are already a 3-way switch. A third resource with `--type` will copy this again.

**Direction:** One field, e.g. `TypeMode none | function | trigger`, and one place that binds `--type` and reads it.

**What:** Help still describes triggers as webhook-only.

**Where:** `src/cli/trigger.go` — parent `Short`, `create.Short`, `create.Example`

**Why:** Init examples include error-handler; parent/create help does not. `--error-handler-path` is easy to miss. `annotateRequiredHelp` will show `(required: --webhook or --error-handler-path)` on create, but Short still says “links a webhook.”

**Direction:** Short: webhook or error-handler → server function. Add one create example with `--error-handler-path`.

### Low

**What:** Generic `ScaffoldJSONC(TypeTrigger, …)` still hard-codes webhook.

**Where:** `src/glide/scaffold.go` — `ScaffoldJSONC` `case TypeTrigger`

**Why:** CLI init no longer uses this for triggers, but `TestScaffoldJSONCLoadsAsDeployable` and any future caller get webhook-only with no error. Easy to regress.

**Direction:** Remove the TypeTrigger case (force `ScaffoldTriggerJSONC`) or require a source argument.

**What:** CLI aliases `error_handler` / `errorhandler` are only unit-tested in Glide.

**Where:** `tests/cli/resource_init_test.go` — `TestTriggerInitWebhookAndErrorHandler`; `src/glide/scaffold.go` — `NormalizeTriggerSource`

**Why:** pflag `triggerTypeValue.Set` uses the same helper, so it should work, but a CLI invocation is cheap insurance.

**Direction:** One extra `run("trigger", "init", "--name", "x", "--type", "error_handler")` in the existing test.

**What:** `triggerTypeValue` lives in `resource_init.go` while `functionTypeValue` lives in `resources.go`.

**Where:** `src/cli/resource_init.go` vs `src/cli/resources.go`

**Why:** Same pflag pattern, two homes.

**Direction:** Keep both next to each other (or one small `enumFlag` helper). No behavior change.

## 3. Suggested order of work

1. **Glide source-kind detection + tests** (`triggerSourceDestChanged`, error-handler push). Do this before telling people to `init --type error-handler` and `deploy push`.
2. **Same change set:** `findRemoteTrigger` matching on `errorHandler.path`.
3. **`runTriggerCreate` waitForResponse** + create test. Same review as (1) if you are already in the trigger payload.
4. **`--snippet` vs `--type`** in `runResourceInit` (behavior choice — see below).
5. **Batch:** help Short/examples, `TypeMode` instead of two bools, `ScaffoldJSONC` TypeTrigger case, alias CLI test.

## 4. Needs human decision

- **`--type` with `--snippet`:** keep required and validate source shape, or drop `--type` when `--snippet` is set. Second option is a flag-requirement change.
- **`trigger create --error-handler-path` and `waitForResponse`:** omit when unset (match init/docs) vs keep sending `true` (current CLI). Anyone already using create with error-handler would see a body change.
- **Omit `waitForResponse` vs `"waitForResponse": false` in error-handler JSONC.** Omit matches the new docs; explicit false may compare more cleanly against platform GETs that always include the field. `triggerComparable` only copies the key when present, so omit vs false can look like a drift.

## 5. Out of scope

- staticcheck: unused `tuiRewrotePrompt` (`src/cli/tui.go`), unused `label` after reassignment in `printResults` (`src/cli/glide.go:349`), unused `nonInteractive` / `wrapConfig` / `resourceLabel` / `fmtFail`, S1017 in `src/projinit/source.go`, unused `stdout` in `tests/cli/generate_test.go`. None of these are on the trigger-init path. Grep before deleting: `Forbidden`, `NewMemoryClient`, `PrintOk`, `MissingCommand`, etc. are used from `tests/` (deadcode starts from main, not external test packages).
- `printOk` on stdout is existing CLI convention; leaving it.
- POLY-CLI-14/26/27/28, Homebrew, Java, SDK adapter git.

## 6. Signals

| Command | Result |
| --- | --- |
| `go build ./...` | pass |
| `go vet ./...` | pass |
| `go test -race ./...` | pass (`tests/cli` 12.6s; all test packages ok) |
| `golangci-lint run` | not configured / not on PATH; used staticcheck instead |
| `go run honnef.co/go/tools/cmd/staticcheck@latest ./...` | 8 findings, none in trigger-init files (see Out of scope) |
| `go run golang.org/x/tools/cmd/deadcode@latest ./...` | reported exported test helpers + a few unused internals; **no** hits in `resource_init.go` / `scaffold.go` / `trigger.go` |
