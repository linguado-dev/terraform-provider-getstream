# Local testing & validation

Two ways to exercise the provider locally. Neither commits secrets: real
credentials live in a gitignored `.env` (copied from `.env.template`).

## Credentials

```bash
cp .env.template .env      # fill in a THROWAWAY / dev app's key + secret
set -a && source .env && set +a
```

The four variables (`STREAM_API_KEY`, `STREAM_API_SECRET`, `STREAM_APP_NAME`,
`STREAM_APP_ID`) are read by the provider directly, so a bare
`provider "getstream" {}` block works. `app_name` activates the wrong-app guard.

> ⚠️ Acceptance tests and `terraform apply` perform **destructive CRUD** (create +
> delete channel types, which are app-global). Only ever point them at a
> throwaway/dev app.

## Option 1: Go acceptance tests (idiomatic)

Drives real create/update/import/delete through the plugin framework, with
`ImportStateVerify` and `CheckDestroy`. This is the authoritative integration test.

```bash
make testacc                                  # all acceptance tests
make testacc TESTARGS='-run TestAccChannelType'   # one test
make test                                     # unit tests only (no creds)
```

## Option 2: `terraform plan` via dev_overrides (eyeball a real plan)

Useful to confirm the `Optional+Computed` defaults don't produce a perpetual diff.

1. Build the binary into the overrides dir:
   ```bash
   make install-dev
   ```
2. Point Terraform at it (`~/.terraformrc`):
   ```hcl
   provider_installation {
     dev_overrides {
       "linguado-dev/getstream" = "/Users/<you>/.terraform.d/plugins-dev-getstream"
     }
     direct {}
   }
   ```
3. In a scratch dir with a `getstream` config, run `terraform plan`. With
   `dev_overrides`, **skip `terraform init`** — Terraform uses the local binary and
   prints a warning. `plan` is read-only; `apply` mutates the app.
