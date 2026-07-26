# HANDOFF — linguado-dev/terraform-provider-getstream

_Last updated: 2026-07-26. Working notes, not user-facing docs._

## ✅ M1 shipped + tested live (this session, 2026-07-26)
`getstream_channel_type` resource, `getstream_app` data source, provider
credential/guard hardening, tests, CI, docs — all validated with **live acceptance
tests (real CRUD) against the dev throwaway app**. See the detailed sections below.
Local Go is now 1.26.5 — no Docker needed; use the `GNUmakefile` targets.
- Env vars are `STREAM_API_KEY` / `STREAM_API_SECRET` / `STREAM_APP_NAME` /
  `STREAM_APP_ID` (one word). Local creds live in a gitignored `.env` (copy from
  `.env.template`); `TestMain` auto-loads it via godotenv (real env wins over `.env`).
- Test tiers: `make test` (unit only, forces `TF_ACC=`), `make verify-creds`
  (non-destructive cred+guard check), `make testacc` (full live CRUD). `make check`
  = pre-PR gate (fmt/vet/build/unit).
- **Two GitHub Environments** (`dev`, `prod`) hold the app secrets/variables. dev =
  any branch (acc-dev runs on PRs); prod = protected + required reviewer
  (nikolaus-linguado), acc-prod runs on release. CI: `.github/workflows/test.yml`
  (build/generate/unit always; acc-dev on PR/push; acc-prod on release). `concurrency`
  + `max-parallel:1` serialize CRUD against the shared apps.
- **app_id guard reality** (see `APP_ID_FINDINGS.md`): numeric app_id only reachable
  via account-tier "ampere" API with a dashboard session token (not CI-injectable),
  so the provider verifies `app_name` (from `GetAppSettings`) instead. `app_id` is a
  documented passthrough. Future: account-tier resources gated on an account token.
- **No `STREAM_API_REGION`**: the `stream-chat-go` v6 SDK reads `STREAM_KEY`/
  `STREAM_SECRET`, `STREAM_CHAT_URL` (base-URL override) and `STREAM_CHAT_TIMEOUT`
  — there is no region setting (Chat is a single global endpoint, routed by API
  key). Region/EU-residency would be a `base_url`/`STREAM_CHAT_URL` attribute, not a
  region string. **Future enhancement:** a GitHub Project for multi-region testing
  (base_url attribute + EU endpoint) — not required now.


## What this repo is
Hard fork (clean break) of the abandoned `talesporto/terraform-provider-getstreamio`,
re-homed under `linguado-dev` to become the maintained supplier of a GetStream.io
Terraform provider. Goal: manage GetStream **app config** (channel types, app
settings, push, roles) as IaC with `terraform plan` drift-detection across linguado
dev/qa/prod. Full plan: `~/github.com/getstream-terraform-provider-plan.md`
(tracking `linguado-backend#979`).

## ✅ Done so far
- **Re-home** (commit `893f620`): public repo, module path renamed to
  `github.com/linguado-dev/terraform-provider-getstream`, registry Address
  `registry.terraform.io/linguado-dev/getstream`, `ATTRIBUTION.md` (MPL-2.0), no
  upstream remote.
- **M0 — framework migration** (commit `29141cf`): ported from the legacy
  `terraform-plugin-framework` v0.9 API to **v1.19.0**. Build + vet + `go test` all green.
  - `provider.go`: `provider.Provider` with `Metadata`/`Schema`/`Configure`/`Resources`/`DataSources`. Provider type name = **`getstream`**.
  - `sqs_resource.go`: `resource.Resource` (Metadata/Schema/Configure/CRUD/ImportState) — **this is the reference pattern for every new resource.**
  - Dropped the abandoned `dcarbone/terraform-plugin-framework-utils`; replaced with a small in-repo `urlValidator` (bottom of `sqs_resource.go`).
  - Resource renamed `getstreamio_sqs` → **`getstream_sqs`**.

## ⚠️ Repo conventions (don't trip on these)
- **Push identity:** origin uses SSH alias `github.com-nikolaus_linguado` (the
  linguado key), NOT the default github.com key. `git remote -v` should show
  `git@github.com-nikolaus_linguado:linguado-dev/...`. The `.gh-user` in
  `~/github.com/linguado-dev/` makes `gh` use `nikolaus-linguado` automatically.
- **Build/test in Docker** (local Go may be too old): 
  `docker run --rm -v $PWD:/w -v /tmp/gocache-gs:/root/.cache/go-build -v /tmp/gomod-gs:/go/pkg/mod -w /w golang:1.26 sh -c "go build ./... && go vet ./... && go test ./..."`
  (caches are warm from this session.)
- `go.mod`: `go 1.25.0`, framework `v1.19.0`, `stream-chat-go/v6 v6.0.0`.

## ✅ M1 — `getstream_channel_type` (implemented + live-CRUD verified)
Motivation: the GetStream audit found real dev↔prod drift (a `room_chat` channel
type in prod but not dev). This resource makes channel types declarative.

Implemented `internal/provider/channel_type_resource.go` and registered
`NewChannelTypeResource` in `provider.go`. Unit tests + live acceptance tests
(create/update/import/delete) pass against the dev throwaway app; the
`Optional+Computed` defaults held (no perpetual diff). Implementation notes:
- `name` is `Required` + `RequiresReplace` (rename = destroy+create); import id = name
  (the resource has no `id` attr, so the acc test sets `ImportStateVerifyIdentifierAttribute: "name"`).
- All config fields are `Optional+Computed` + `UseStateForUnknown` so unset fields
  take GetStream server defaults without perpetual diffs (verified live).
- Create uses `stream.NewChannelType(name)` (seeds `DefaultChannelConfig`) then
  overlays only known config values; Update sends a changed-fields `map` then
  re-GETs (Update returns no body). Read 404 → `RemoveResource` via `isNotFound`
  (`errors.As` on `stream.Error` value, `StatusCode == 404`).
- **SDK gotcha:** `Automod`/`ModBehavior`/`BlockListBehavior` are unexported types
  (`modType`/`modBehaviour`) — assign via exported constants (`stream.AutoModSimple`,
  `stream.ModBehaviourFlag`, …) in Create; Update's map takes raw strings. Enum
  validation is shared between Create and Update (`validateAutomod`/`validateBehavior`).

### Original M1 spec (kept for reference)

**SDK surface (verified this session, `GetStream/stream-chat-go/v6`):**
```go
func (c *Client) CreateChannelType(ctx, *ChannelType) (*ChannelTypeResponse, error)
func (c *Client) GetChannelType(ctx, name string) (*GetChannelTypeResponse, error)
func (c *Client) UpdateChannelType(ctx, name string, options map[string]interface{}) (*Response, error)
func (c *Client) DeleteChannelType(ctx, name string) (*Response, error)
func (c *Client) ListChannelTypes(ctx) (*ChannelTypesResponse, error)   // for the data source later
```
Note the asymmetry: **Create takes a `*ChannelType` struct; Update takes a
`map[string]interface{}`** of changed fields. `id` = the channel type `name`
(make it `Required` + ForceNew via `RequiresReplace` plan modifier — renaming a
channel type is a destroy+create).

**Schema = `ChannelType` → embeds `ChannelConfig` (verified fields):**
- string: `name` (id), `message_retention`, `automod`, `automod_behavior` (json `automod_behavior`), `blocklist`, `blocklist_behavior`
- bool: `typing_events`, `read_events`, `connect_events`, `search`, `reactions`, `reminders`, `replies`, `mutes`, `push_notifications`, `uploads`, `url_enrichment`, `custom_events`
- int: `max_message_length`
- also on `ChannelType` (not `ChannelConfig`): `commands []string`, `grants map[string][]string` (list/map attributes — do these after the scalars build)
- Mark server-defaulted fields `Computed: true` + `UseStateForUnknown()` where GetStream fills them, else plan will show perpetual diffs. **Verify actual defaults against a live app before finalizing Computed vs Optional.**

**Read** should `GetChannelType(name)`; if it 404s, `resp.State.RemoveResource(ctx)`
(drift/out-of-band delete) — mirror how a real provider handles a missing resource.
`ChannelType` has no exported "not found" helper; inspect the error
(`stream.APIError`/status) — confirm the shape when wiring Read.

**Verify (verify-first, no live app needed for the compile gate):**
`go build ./... && go vet ./...` in the golang:1.26 container. Full CRUD needs a
real Stream **dev** app (see below) with `TF_ACC=1`.

## Integration testing (when ready)
- Create/point at the linguado **dev** GetStream app; export `STREAM_API_KEY` /
  `STREAM_API_SECRET` (throwaway/dev only, never commit).
- `dev_overrides` for local iteration: `~/.terraformrc`
  ```hcl
  provider_installation {
    dev_overrides { "linguado-dev/getstream" = "<dir with the built binary>" }
    direct {}
  }
  ```
  Then `go build -o <dir>/terraform-provider-getstream` and `terraform plan` in a
  scratch config. (dev_overrides warns on `apply` and skips `init` — dev only.)

## Later milestones (see plan file)
M2 `getstream_app_settings` (singleton; read-modify-write, nested objects **replace** not merge) · M3 data sources · M4 push_provider + role · M6 **publish to Terraform Registry** as `linguado-dev/getstream` (needs GPG-signed release tags + goreleaser + registry.terraform.io namespace registration → then linguado consumes via `required_providers { getstream = { source = "linguado-dev/getstream" } }`).

## Housekeeping still open
- README.md / docs/ / examples/ still say `getstreamio` / `talesporto` — update when convenient (not blocking).
- `.github/` has no CI yet — add `go test` + `golangci-lint` + `tfplugindocs` workflow before/at publish.
