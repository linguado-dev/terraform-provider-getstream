# Terraform Provider for GetStream.io

[![Registry](https://img.shields.io/badge/registry-linguado--dev%2Fgetstream-blueviolet)](https://registry.terraform.io/providers/linguado-dev/getstream/latest)

Manage [GetStream.io](https://getstream.io) application configuration —
channel types, app settings, and SQS event delivery — as infrastructure-as-code.
`getstream_channel_type` refreshes from the API on read for `terraform plan` drift
detection; `getstream_sqs` cannot (GetStream does not return the SQS secret, so its
state is kept as-is rather than refreshed).

Published to the Terraform Registry as **`linguado-dev/getstream`**. This is a
maintained hard fork of the abandoned `talesporto/terraform-provider-getstreamio`
(see [ATTRIBUTION.md](./ATTRIBUTION.md)).

## Using the provider

```hcl
terraform {
  required_providers {
    getstream = {
      source  = "linguado-dev/getstream"
      version = "~> 0.1.0"
    }
  }
}

provider "getstream" {
  # api_key / api_secret may be set here, or via the STREAM_API_KEY and
  # STREAM_API_SECRET environment variables (STREAM_KEY / STREAM_SECRET, the
  # Go SDK names, are also accepted).
  app_name = "my-app" # optional wrong-app guard (also STREAM_APP_NAME)
}

resource "getstream_channel_type" "messaging" {
  name               = "messaging"
  reactions          = true
  replies            = true
  max_message_length = 5000
  automod            = "disabled"
}

# Assert you are pointed at the intended app.
data "getstream_app" "this" {}
```

### Authentication

Credentials resolve in this order (first non-empty wins):

1. Provider `api_key` / `api_secret` attributes.
2. `STREAM_API_KEY` / `STREAM_API_SECRET` env vars (or `STREAM_KEY` / `STREAM_SECRET`).
3. Terraform input variables via `TF_VAR_` (see `examples/provider`).

The optional `app_name` attribute (or `STREAM_APP_NAME`) makes the provider verify
the credentials resolve to an app with that name, guarding against pointing an
environment's Terraform at the wrong app.

### Resources & data sources

| Name | Kind | Description |
|---|---|---|
| `getstream_channel_type` | resource | Channel type + config (feature toggles, retention, automod, blocklist, commands, grants). Full CRUD + import. |
| `getstream_sqs` | resource | SQS event-delivery link on the app. |
| `getstream_app` | data source | App name + organization, for `precondition` assertions. |

Full reference docs are on the
[Terraform Registry](https://registry.terraform.io/providers/linguado-dev/getstream/latest/docs).

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.25 (to build)

## Developing the provider

All commands are schematized in the `GNUmakefile`:

```shell
make build         # go build
make test          # unit tests (no credentials)
make verify-creds  # non-destructive credential + wrong-app-guard check (needs .env)
make testacc       # full acceptance tests — real, destructive CRUD (needs .env)
make docs          # regenerate registry docs (tfplugindocs)
make check         # pre-PR gate: fmt + vet + build + unit tests
```

For local acceptance runs, copy `.env.template` to `.env` (gitignored) and fill in a
**throwaway/dev** app's credentials — see [`docs-dev/local-testing.md`](./docs-dev/local-testing.md).

> ⚠️ Acceptance tests and `terraform apply` perform destructive CRUD against the app
> the credentials resolve to. Only ever point them at a non-production app.

## Releasing

Push a `v*` tag on `main`; `.github/workflows/release.yml` runs GoReleaser to build,
GPG-sign, and publish the release, which the Terraform Registry ingests
automatically.

## License

[MPL-2.0](./LICENSE).
