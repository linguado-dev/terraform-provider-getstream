## 0.3.1

BUG FIXES:

* provider: declaring the provider without credentials is no longer a configure-time error. Terraform configures every declared provider even when an environment manages zero GetStream resources (`for_each = {}`), which broke every un-wired environment's plan (linguado-infra qa/prod since #97). The "Missing GetStream.io api_key/api_secret" error is now deferred to the first resource or data source that actually needs the client; environments with credentials behave exactly as before (eager validation + app-name guard).

## 0.1.0 (Unreleased)

FEATURES:
