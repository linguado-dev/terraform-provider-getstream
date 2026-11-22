## 0.3.2

BUG FIXES:

* resource/getstream_channel_type: a refresh right after `apply` no longer reports a spurious diff from GetStream's eventually consistent reads ([#17](https://github.com/linguado-dev/terraform-provider-getstream/issues/17)). Create and Update record the write time in private state; a Read within two minutes of it polls (bounded, about 15 seconds) until the API returns what was written, and a not-found is retried the same way. Reads after that window, and reads of imported resources, stay single-shot, so real drift shows up as before.

## 0.3.1

BUG FIXES:

* provider: declaring the provider without credentials is no longer a configure-time error. Terraform configures every declared provider even when an environment manages zero GetStream resources (`for_each = {}`), which broke every un-wired environment's plan (linguado-infra qa/prod since #97). The `Missing GetStream.io API key` / `Missing GetStream.io API secret` diagnostics are unchanged in wording but are now raised by the first resource or data source that actually needs the client instead of at configure time; environments with credentials behave exactly as before (eager validation + app-name guard).

## 0.1.0 (Unreleased)

FEATURES:
