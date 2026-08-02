package provider

import (
	"testing"

	stream "github.com/GetStream/stream-chat-go/v6"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCommandFromModel_OnlyKnownFields(t *testing.T) {
	t.Parallel()
	data := commandResourceModel{
		Name:        types.StringValue("giphy"),
		Description: types.StringValue("Post a gif"),
		// Args + Set null -> must remain empty on the request
	}
	cmd := commandFromModel(data)
	if cmd.Name != "giphy" {
		t.Errorf("Name = %q", cmd.Name)
	}
	if cmd.Description != "Post a gif" {
		t.Errorf("Description = %q", cmd.Description)
	}
	if cmd.Args != "" || cmd.Set != "" {
		t.Errorf("unset fields leaked: args=%q set=%q", cmd.Args, cmd.Set)
	}
}

func TestMapCommandToModel_RoundTrip(t *testing.T) {
	t.Parallel()
	cmd := &stream.Command{Name: "ban", Description: "Ban a user", Args: "[@username]", Set: "moderation_set"}
	var data commandResourceModel
	mapCommandToModel(cmd, &data)
	if data.Name.ValueString() != "ban" {
		t.Errorf("Name = %q", data.Name.ValueString())
	}
	if data.Description.ValueString() != "Ban a user" {
		t.Errorf("Description = %q", data.Description.ValueString())
	}
	if data.Args.ValueString() != "[@username]" {
		t.Errorf("Args = %q", data.Args.ValueString())
	}
	if data.Set.ValueString() != "moderation_set" {
		t.Errorf("Set = %q", data.Set.ValueString())
	}
}

func TestMapCommandToModel_NilNoop(t *testing.T) {
	t.Parallel()
	data := commandResourceModel{Name: types.StringValue("keep")}
	mapCommandToModel(nil, &data)
	if data.Name.ValueString() != "keep" {
		t.Errorf("nil command should not mutate model; got %q", data.Name.ValueString())
	}
}
