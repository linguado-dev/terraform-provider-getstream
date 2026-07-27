# App settings are a singleton — declare one getstream_app_settings per app/provider.
# You manage only the fields you set; unset fields are left untouched on the app.
resource "getstream_app_settings" "this" {
  # Event delivery (secrets are write-only; not read back from the API).
  sqs_url    = "https://sqs.us-east-1.amazonaws.com/000000000000/getstream-events"
  sqs_key    = "AKIAEXAMPLE"
  sqs_secret = var.sqs_secret

  # Webhooks.
  webhook_url    = "https://api.example.com/getstream/webhook"
  webhook_events = ["message.new", "channel.created"]

  # Upload rules.
  file_upload_config = {
    blocked_file_extensions = [".exe", ".sh"]
  }

  # Behavior toggles.
  async_url_enrich_enabled = true
  reminders_interval       = 60
}

variable "sqs_secret" {
  type      = string
  sensitive = true
  default   = null
}
