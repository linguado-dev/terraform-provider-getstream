package provider

import (
	"context"
	"fmt"
	"testing"

	stream "github.com/GetStream/stream-chat-go/v6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// acctestChannelTypeName is a fixed, provider-specific channel type name used by
// the acceptance test. It is created and destroyed within the test.
const acctestChannelTypeName = "tf_acctest_channel_type"

// TestAccChannelTypeResource exercises the full lifecycle (create, read, update,
// import) against a live GetStream.io app. It runs only under TF_ACC with
// credentials set, and cleans up after itself via CheckDestroy.
func TestAccChannelTypeResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckChannelTypeDestroy,
		Steps: []resource.TestStep{
			// Create + Read.
			{
				Config: testAccChannelTypeConfig(true, 1000, "simple"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("getstream_channel_type.test", "name", acctestChannelTypeName),
					resource.TestCheckResourceAttr("getstream_channel_type.test", "reactions", "true"),
					resource.TestCheckResourceAttr("getstream_channel_type.test", "max_message_length", "1000"),
					resource.TestCheckResourceAttr("getstream_channel_type.test", "automod", "simple"),
				),
			},
			// Import. The resource's identifier is "name" (there is no "id"
			// attribute), so verification keys off that.
			{
				ResourceName:                         "getstream_channel_type.test",
				ImportState:                          true,
				ImportStateId:                        acctestChannelTypeName,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
			// Update + Read.
			{
				Config: testAccChannelTypeConfig(false, 2000, "disabled"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("getstream_channel_type.test", "reactions", "false"),
					resource.TestCheckResourceAttr("getstream_channel_type.test", "max_message_length", "2000"),
					resource.TestCheckResourceAttr("getstream_channel_type.test", "automod", "disabled"),
				),
			},
			// Delete happens automatically at the end of the test.
		},
	})
}

func testAccChannelTypeConfig(reactions bool, maxLen int, automod string) string {
	return fmt.Sprintf(`
provider "getstream" {}

resource "getstream_channel_type" "test" {
  name               = %[1]q
  reactions          = %[2]t
  max_message_length = %[3]d
  automod            = %[4]q
}
`, acctestChannelTypeName, reactions, maxLen, automod)
}

// testAccCheckChannelTypeDestroy verifies the channel type no longer exists after
// the test tears down, using a client built from the same env credentials.
func testAccCheckChannelTypeDestroy(s *terraform.State) error {
	client, err := stream.NewClient(firstEnv(envAPIKeyNames), firstEnv(envAPISecretNames))
	if err != nil {
		return fmt.Errorf("building verification client: %w", err)
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "getstream_channel_type" {
			continue
		}
		_, err := client.GetChannelType(context.Background(), rs.Primary.Attributes["name"])
		if err == nil {
			return fmt.Errorf("channel type %q still exists after destroy", rs.Primary.Attributes["name"])
		}
		if !isNotFound(err) {
			return fmt.Errorf("unexpected error checking channel type %q: %w", rs.Primary.Attributes["name"], err)
		}
	}
	return nil
}
