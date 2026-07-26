provider "getstream" {
  # Credentials may also be supplied via the GETSTREAM_KEY and
  # GETSTREAM_SECRET environment variables.
  api_key    = var.getstream_api_key
  api_secret = var.getstream_api_secret

  # Optional wrong-app guard: fail fast if these credentials do not resolve to
  # an app with this name (e.g. a prod key pasted into a non-prod config).
  app_name = "linguado-dev"

  # Optional, for documentation/cross-reference only (not verified).
  app_id = "1699146"
}
