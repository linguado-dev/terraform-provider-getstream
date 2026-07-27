package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccAppSettingsResource exercises the app-settings singleton against a live
// app. It toggles low-risk, non-destructive fields (async URL enrichment and the
// reminders interval) rather than event-delivery/webhook config, so it does not
// disrupt real integrations on the shared dev app.
func TestAccAppSettingsResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAppSettingsConfig(true, 60),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("getstream_app_settings.test", "async_url_enrich_enabled", "true"),
					resource.TestCheckResourceAttr("getstream_app_settings.test", "reminders_interval", "60"),
					resource.TestCheckResourceAttrSet("getstream_app_settings.test", "id"),
				),
			},
			{
				// Import establishes management of the singleton by id. Because this
				// resource manages only the subset of settings declared in config,
				// imported state legitimately differs from a full config (unmanaged
				// fields are null), so ImportStateVerify is not meaningful here — we
				// assert the import succeeds and yields the fixed id.
				ResourceName:  "getstream_app_settings.test",
				ImportState:   true,
				ImportStateId: appSettingsID,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 imported state, got %d", len(states))
					}
					if got := states[0].Attributes["id"]; got != appSettingsID {
						return fmt.Errorf("imported id = %q, want %q", got, appSettingsID)
					}
					return nil
				},
			},
			{
				Config: testAccAppSettingsConfig(false, 120),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("getstream_app_settings.test", "async_url_enrich_enabled", "false"),
					resource.TestCheckResourceAttr("getstream_app_settings.test", "reminders_interval", "120"),
				),
			},
		},
	})
}

func testAccAppSettingsConfig(asyncEnrich bool, remindersInterval int) string {
	return fmt.Sprintf(`
provider "getstream" {}

resource "getstream_app_settings" "test" {
  async_url_enrich_enabled = %[1]t
  reminders_interval       = %[2]d
}
`, asyncEnrich, remindersInterval)
}
