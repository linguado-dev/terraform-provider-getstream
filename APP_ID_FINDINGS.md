# Findings: app identity, `GetApp`, and the app_id gap

Notes from wiring GetStream into linguado (2026-07-26). Relevant to what this
provider should expose so consumers can validate *which* app a key targets.

## The problem we hit
Linguado runs three GetStream apps (per-env isolation):

| Env  | app_id  | app name       |
|------|---------|----------------|
| dev  | 1699146 | `linguado-dev` |
| qa   | 1699202 | `linguado-qa`  |
| prod | 1407211 | `Linguado`     |

Each env's backend gets its own `STREAM_API_KEY` / `STREAM_API_SECRET` (GitHub
Environment secrets) + a `STREAM_APP_ID` **variable** for documentation/cross-check.
The footgun we want CI/Terraform to catch: **the prod key pasted into a non-prod env**
(a leaked/mixed-up dev key that can actually reach prod chat data).

## The gap: the app_id is not readable from the app-scoped API
- Server auth is `api_key : api_secret` (the machine credential). The **key does NOT
  embed the numeric app_id** — it's a random alphanumeric handle.
- `GET /app` (SDK `GetAppSettings` → `AppResponse.App AppSettings`) returns
  `name` + `organization` (`OrganizationName`) but **NO `app_id` / `app_cid`** field.
  Verified against `GetStream/stream-chat-go` `app.go` (master, 2026-07).
- So from key+secret alone you can recover the app **name** and **org**, but **not the
  numeric app_id**. There is no app-scoped endpoint that echoes the id back.

### Consequence for validation
- **CI (linguado) workaround:** maintain a hardcoded `app_id → name` map, call
  `GET /app` with the env's key+secret, and assert the returned `.name` equals the
  expected name for that env's `STREAM_APP_ID`. Indirect (name proxy), but it
  catches the prod-key-in-nonprod case. This is a stopgap.
- **This provider is the right home for the real fix.** Because a `provider "getstream"`
  block is configured *with* `api_key`/`api_secret` **and** (ideally) an explicit
  `app_id`, it can validate the binding at plan/apply — no name-map hack needed.

## What this provider should do about it
1. **Provider schema:** accept an optional `app_id` (number/string) alongside
   `api_key` + `api_secret`. If set, validate it at configure-time.
2. **Validation options (pick what the API supports — verify against a live app):**
   - The **account/management API** (org-level, different from the app-scoped chat API)
     *does* know app_ids — `GetApp` at the account tier lists apps with their ids.
     If the provider can reach that (with an account token, not just app key+secret),
     it can confirm `api_key` belongs to the declared `app_id`.
   - Failing that, expose the recoverable identity as a **data source**
     `data "getstream_app"` returning `name` + `organization` (what `GET /app` gives),
     so a consumer can `precondition` on the name — the same name-proxy check, but
     first-class in TF instead of a CI curl.
3. **Minimum viable:** even just surfacing `name`/`organization` via a data source is
   more than the CLI/API gives ergonomically today, and lets linguado replace the CI
   name-map hack with a `terraform plan` assertion.

## RESOLVED (2026-07-25): account-tier API captured from the beta dashboard HAR
Captured `beta.dashboard.getstream.io.har` (loading the app page). Findings:

- The **account-tier "ampere" API** on `getstream.io/api/ampere/...` *does* expose app_id:
  - `POST /api/ampere/app/read` body `{"org_id":1286773,"app_id":1699146}` →
    `{"items":[{"id":1699146,"org_id":1286773,"name":"linguado-dev","is_development":true,...}]}`
    (omit `app_id` to list all apps in the org).
  - `POST /api/ampere/app/access_key/read` body `{"app_id":1699146}` →
    `{"items":[{"id":...,"key":"ad6tp4zk8uut","secret":"<...>",...}]}` — **the exact
    `app_id → key/secret` binding** we'd want to assert.
  - `POST /api/ampere/organization/read` `{"org_id":1286773}` → org name/slug/email.
- **Auth is the blocker:** these use header `x-access-token: <86-char opaque token>`
  (NOT a JWT, NOT the app api_key/secret). It's an **account/org session token**
  (org-scoped to 1286773), issued by interactive dashboard login — no token-mint or
  login endpoint appears in the capture. So it is a *different auth tier* from the
  app-scoped chat API the Go SDK speaks, and not obviously CI-injectable.

### Verdict
- **Direct `api_key ↔ app_id` validation is possible** (via `access_key/read`) **but only
  with an account access token**, which we cannot derive from the app key+secret and
  (as captured) requires dashboard login. Open follow-up: does Stream offer a
  non-interactive **personal/account access token** that CI could hold? If yes → the
  provider can do the real binding check. If no → account-tier validation stays manual.
- **What the provider CAN do today with just app key+secret:** recover app **name** +
  **organization** via `GetAppSettings` (`GET /app`). That supports the **name-proxy**
  guard (assert returned `name` == expected for this env), which catches the
  prod-key-in-nonprod footgun without an account token.

Linguado's live app_id ↔ name map (for the name-proxy check):
| Env  | app_id  | name           |
|------|---------|----------------|
| dev  | 1699146 | `linguado-dev` |
| qa   | 1699202 | `linguado-qa`  |
| prod | 1407211 | `Linguado`     |

## TODO when building the provider
- [ ] Decide guard design (see below) — name-proxy now vs. account-token direct check later.
- [ ] Optionally ship `data "getstream_app"` (name/org from `GET /app`) so consumers can
      `precondition` on the name in TF instead of the CI curl name-map hack.
- [ ] Add an example showing per-env provider blocks asserting the right app.
- [ ] Follow-up: investigate a non-interactive account/personal access token for the
      ampere API — that's the unlock for true `api_key ↔ app_id` validation.

Refs: linguado-backend#977 (vendor isolation), #979 (this provider spec), the CI
name-map cross-check added to `linguado-backend/.github/workflows/_deploy.yml`.
