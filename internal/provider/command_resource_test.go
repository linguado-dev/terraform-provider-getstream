package provider

import (
	"context"
	"fmt"
	"testing"

	stream "github.com/GetStream/stream-chat-go/v6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const acctestCommandName = "tf_acctest_command"

// TestAccCommandResource exercises the full command lifecycle (create, read,
// update, import) against a live app, cleaning up via CheckDestroy.
func TestAccCommandResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCommandDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccCommandConfig("First description", "[arg1]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("getstream_command.test", "name", acctestCommandName),
					resource.TestCheckResourceAttr("getstream_command.test", "description", "First description"),
					resource.TestCheckResourceAttr("getstream_command.test", "args", "[arg1]"),
				),
			},
			{
				ResourceName:                         "getstream_command.test",
				ImportState:                          true,
				ImportStateId:                        acctestCommandName,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
			{
				Config: testAccCommandConfig("Updated description", "[arg2]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("getstream_command.test", "description", "Updated description"),
					resource.TestCheckResourceAttr("getstream_command.test", "args", "[arg2]"),
				),
			},
		},
	})
}

func testAccCommandConfig(description, args string) string {
	return fmt.Sprintf(`
provider "getstream" {}

resource "getstream_command" "test" {
  name        = %[1]q
  description = %[2]q
  args        = %[3]q
}
`, acctestCommandName, description, args)
}

func testAccCheckCommandDestroy(s *terraform.State) error {
	client, err := stream.NewClient(firstEnv(envAPIKeyNames), firstEnv(envAPISecretNames))
	if err != nil {
		return fmt.Errorf("building verification client: %w", err)
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "getstream_command" {
			continue
		}
		_, err := client.GetCommand(context.Background(), rs.Primary.Attributes["name"])
		if err == nil {
			return fmt.Errorf("command %q still exists after destroy", rs.Primary.Attributes["name"])
		}
		if !isNotFound(err) {
			return fmt.Errorf("unexpected error checking command %q: %w", rs.Primary.Attributes["name"], err)
		}
	}
	return nil
}
