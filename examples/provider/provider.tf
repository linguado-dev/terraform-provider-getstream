# Credentials can be supplied three ways (first non-empty wins):
#   1. Provider attributes below.
#   2. Provider env fallback: STREAM_API_KEY / STREAM_API_SECRET (GetStream's
#      documented names; STREAM_KEY / STREAM_SECRET from the Go SDK also work).
#      With these set, a bare `provider "getstream" {}` block is enough.
#   3. Terraform input variables via TF_VAR_ (as shown here): export
#      TF_VAR_getstream_api_key / TF_VAR_getstream_api_secret.
variable "getstream_api_key" {
  type    = string
  default = null
}

variable "getstream_api_secret" {
  type      = string
  sensitive = true
  default   = null
}

provider "getstream" {
  api_key    = var.getstream_api_key
  api_secret = var.getstream_api_secret

  # Optional wrong-app guard: fail fast if these credentials do not resolve to
  # an app with this name (e.g. a prod key pasted into a non-prod config).
  # May also be set via STREAM_APP_NAME.
  app_name = "linguado-dev"

  # Optional, for documentation/cross-reference only (not verified).
  # May also be set via STREAM_APP_ID.
  app_id = "1699146"
}
