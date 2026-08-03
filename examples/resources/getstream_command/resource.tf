# A custom chat slash-command. Reference it by name from a channel type's
# `commands` list to enable it there.
resource "getstream_command" "giphy" {
  name        = "giphy"
  description = "Post a GIF"
  args        = "[text]"
  set         = "giphy_set"
}

resource "getstream_channel_type" "messaging" {
  name     = "messaging"
  commands = [getstream_command.giphy.name]
}

# Import an existing command by its name:
#   terraform import getstream_command.giphy giphy
