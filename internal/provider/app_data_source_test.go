package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccAppDataSource is a non-destructive credential check: it configures the
// provider (which fails on invalid credentials) and reads the app identity back.
// If STREAM_APP_NAME is set, it asserts the resolved app name matches — so a
// green run proves the credentials are valid AND point at the expected app.
func TestAccAppDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `provider "getstream" {}
data "getstream_app" "this" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.getstream_app.this", "name"),
					resource.TestCheckResourceAttrSet("data.getstream_app.this", "organization"),
					checkAppNameMatchesEnv,
				),
			},
		},
	})
}

// checkAppNameMatchesEnv asserts the data source's name equals STREAM_APP_NAME
// when that variable is set (skipped otherwise).
func checkAppNameMatchesEnv(s *terraform.State) error {
	want := os.Getenv(envAppName)
	if want == "" {
		return nil
	}
	rs, ok := s.RootModule().Resources["data.getstream_app.this"]
	if !ok {
		return fmt.Errorf("data.getstream_app.this not found in state")
	}
	if got := rs.Primary.Attributes["name"]; got != want {
		return fmt.Errorf("app name mismatch: credentials resolve to %q, expected %q (%s)", got, want, envAppName)
	}
	return nil
}
