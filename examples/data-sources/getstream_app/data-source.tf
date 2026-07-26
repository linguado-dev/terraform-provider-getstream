# Read back the identity of the app the configured credentials resolve to.
data "getstream_app" "this" {}

# Assert Terraform is pointed at the intended app before making changes. This is
# a first-class alternative to the provider's app_name guard when you want the
# check attached to a specific resource or output.
resource "getstream_channel_type" "messaging" {
  name = "messaging"

  lifecycle {
    precondition {
      condition     = data.getstream_app.this.name == "linguado-dev"
      error_message = "Refusing to apply: credentials resolve to app '${data.getstream_app.this.name}', not 'linguado-dev'."
    }
  }
}

output "getstream_app_name" {
  value = data.getstream_app.this.name
}

output "getstream_app_organization" {
  value = data.getstream_app.this.organization
}
