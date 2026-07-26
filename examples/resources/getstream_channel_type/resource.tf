resource "getstream_channel_type" "messaging" {
  name = "messaging"

  # Feature toggles (unset fields inherit the GetStream.io server defaults).
  typing_events      = true
  read_events        = true
  reactions          = true
  replies            = true
  uploads            = true
  push_notifications = true

  max_message_length = 5000
  message_retention  = "infinite"

  automod          = "disabled"
  automod_behavior = "flag"

  commands = ["giphy"]

  grants = {
    admin = ["create-channel", "read-channel", "update-channel"]
    user  = ["read-channel"]
  }
}

# Renaming a channel type forces replacement (destroy + create), since the name
# is the resource identifier. Import an existing type by its name:
#   terraform import getstream_channel_type.messaging messaging
