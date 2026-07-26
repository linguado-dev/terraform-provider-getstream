package provider

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"testing"

	stream "github.com/GetStream/stream-chat-go/v6"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestIsNotFound(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", fmt.Errorf("boom"), false},
		{"stream 404", stream.Error{StatusCode: http.StatusNotFound}, true},
		{"stream 500", stream.Error{StatusCode: http.StatusInternalServerError}, false},
		{"wrapped 404", fmt.Errorf("wrap: %w", stream.Error{StatusCode: http.StatusNotFound}), true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isNotFound(tc.err); got != tc.want {
				t.Fatalf("isNotFound(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestKnownString(t *testing.T) {
	t.Parallel()
	if _, ok := knownString(types.StringNull()); ok {
		t.Fatal("null should not be known")
	}
	if _, ok := knownString(types.StringUnknown()); ok {
		t.Fatal("unknown should not be known")
	}
	if s, ok := knownString(types.StringValue("x")); !ok || s != "x" {
		t.Fatalf("value: got %q,%v", s, ok)
	}
}

// TestApplyModelToChannelType_UnsetKeepsDefaults verifies that an all-null model
// leaves the SDK defaults from NewChannelType untouched (so the server fills them),
// and applies no stray zero-values.
func TestApplyModelToChannelType_UnsetKeepsDefaults(t *testing.T) {
	t.Parallel()
	ct := stream.NewChannelType("messaging")
	data := channelTypeResourceModel{Name: types.StringValue("messaging")}

	diags := applyModelToChannelType(context.Background(), data, ct)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	// Defaults from DefaultChannelConfig must survive.
	if ct.MaxMessageLength != 5000 {
		t.Errorf("MaxMessageLength = %d, want 5000 (default preserved)", ct.MaxMessageLength)
	}
	if ct.MessageRetention != stream.MessageRetentionForever {
		t.Errorf("MessageRetention = %q, want %q", ct.MessageRetention, stream.MessageRetentionForever)
	}
	if !ct.PushNotifications {
		t.Errorf("PushNotifications default should remain true")
	}
	if ct.Automod != stream.AutoModDisabled {
		t.Errorf("Automod = %q, want %q", ct.Automod, stream.AutoModDisabled)
	}
}

func TestApplyModelToChannelType_AppliesKnownValues(t *testing.T) {
	t.Parallel()
	ct := stream.NewChannelType("messaging")
	commands, d := types.ListValueFrom(context.Background(), types.StringType, []string{"giphy", "ban"})
	if d.HasError() {
		t.Fatal(d)
	}
	grants, d := types.MapValueFrom(context.Background(), types.ListType{ElemType: types.StringType},
		map[string][]string{"admin": {"create-channel"}})
	if d.HasError() {
		t.Fatal(d)
	}
	data := channelTypeResourceModel{
		Name:              types.StringValue("messaging"),
		TypingEvents:      types.BoolValue(false),
		Reactions:         types.BoolValue(true),
		MaxMessageLength:  types.Int64Value(100),
		MessageRetention:  types.StringValue("7"),
		Automod:           types.StringValue("simple"),
		AutomodBehavior:   types.StringValue("block"),
		Blocklist:         types.StringValue("profanity"),
		BlocklistBehavior: types.StringValue("flag"),
		Commands:          commands,
		Grants:            grants,
	}

	diags := applyModelToChannelType(context.Background(), data, ct)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if ct.TypingEvents {
		t.Error("TypingEvents should be false")
	}
	if !ct.Reactions {
		t.Error("Reactions should be true")
	}
	if ct.MaxMessageLength != 100 {
		t.Errorf("MaxMessageLength = %d, want 100", ct.MaxMessageLength)
	}
	if ct.MessageRetention != "7" {
		t.Errorf("MessageRetention = %q, want 7", ct.MessageRetention)
	}
	if ct.Automod != stream.AutoModSimple {
		t.Errorf("Automod = %q, want simple", ct.Automod)
	}
	if ct.ModBehavior != stream.ModBehaviourBlock {
		t.Errorf("ModBehavior = %q, want block", ct.ModBehavior)
	}
	if ct.BlockList != "profanity" {
		t.Errorf("BlockList = %q, want profanity", ct.BlockList)
	}
	if ct.BlockListBehavior != stream.ModBehaviourFlag {
		t.Errorf("BlockListBehavior = %q, want flag", ct.BlockListBehavior)
	}
	if len(ct.Commands) != 2 || ct.Commands[0].Name != "giphy" || ct.Commands[1].Name != "ban" {
		t.Errorf("Commands = %+v, want [giphy ban]", ct.Commands)
	}
	if got := ct.Grants["admin"]; len(got) != 1 || got[0] != "create-channel" {
		t.Errorf("Grants[admin] = %v, want [create-channel]", got)
	}
}

func TestApplyModelToChannelType_MaxMessageLengthOverflow(t *testing.T) {
	t.Parallel()
	ct := stream.NewChannelType("messaging")
	data := channelTypeResourceModel{
		Name:             types.StringValue("messaging"),
		MaxMessageLength: types.Int64Value(int64(math.MaxInt32) + 1),
	}
	diags := applyModelToChannelType(context.Background(), data, ct)
	if !diags.HasError() {
		t.Fatal("expected diagnostic for max_message_length exceeding 32-bit range")
	}
}

func TestApplyModelToChannelType_InvalidEnumsError(t *testing.T) {
	t.Parallel()
	ct := stream.NewChannelType("messaging")
	data := channelTypeResourceModel{
		Name:            types.StringValue("messaging"),
		Automod:         types.StringValue("bogus"),
		AutomodBehavior: types.StringValue("nope"),
	}
	diags := applyModelToChannelType(context.Background(), data, ct)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid automod/behavior values")
	}
	if diags.ErrorsCount() != 2 {
		t.Errorf("expected 2 errors (automod + automod_behavior), got %d: %v", diags.ErrorsCount(), diags)
	}
}

func TestUpdateOptions_OnlyKnownFields(t *testing.T) {
	t.Parallel()
	data := channelTypeResourceModel{
		Name:             types.StringValue("messaging"),
		Reactions:        types.BoolValue(true),
		MaxMessageLength: types.Int64Value(42),
		Automod:          types.StringValue("AI"),
		// everything else null -> must be absent from the options map
	}
	opts, diags := updateOptions(context.Background(), data)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if v, ok := opts["reactions"].(bool); !ok || !v {
		t.Errorf("reactions = %v, want true", opts["reactions"])
	}
	if v, ok := opts["max_message_length"].(int64); !ok || v != 42 {
		t.Errorf("max_message_length = %v, want 42", opts["max_message_length"])
	}
	if opts["automod"] != "AI" {
		t.Errorf("automod = %v, want AI (raw string for the update map)", opts["automod"])
	}
	// Unset fields must not appear.
	for _, k := range []string{"typing_events", "message_retention", "blocklist", "commands", "grants"} {
		if _, present := opts[k]; present {
			t.Errorf("unset field %q leaked into update options", k)
		}
	}
}

func TestUpdateOptions_InvalidEnumsError(t *testing.T) {
	t.Parallel()
	data := channelTypeResourceModel{
		Name:              types.StringValue("messaging"),
		Automod:           types.StringValue("bogus"),
		AutomodBehavior:   types.StringValue("nope"),
		BlocklistBehavior: types.StringValue("also-bad"),
	}
	opts, diags := updateOptions(context.Background(), data)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid enum values in updateOptions")
	}
	if diags.ErrorsCount() != 3 {
		t.Errorf("expected 3 errors (automod + 2 behaviors), got %d: %v", diags.ErrorsCount(), diags)
	}
	// Invalid values must not be sent to the API.
	for _, k := range []string{"automod", "automod_behavior", "blocklist_behavior"} {
		if _, present := opts[k]; present {
			t.Errorf("invalid %q leaked into update options", k)
		}
	}
}

// TestMapChannelTypeToModel_RoundTrip verifies the API->model hydration covers
// every scalar plus commands/grants.
func TestMapChannelTypeToModel_RoundTrip(t *testing.T) {
	t.Parallel()
	ct := &stream.ChannelType{
		ChannelConfig: stream.ChannelConfig{
			Name:              "team",
			TypingEvents:      true,
			Reactions:         true,
			MaxMessageLength:  256,
			MessageRetention:  "30",
			Automod:           stream.AutoModSimple,
			ModBehavior:       stream.ModBehaviourBlock,
			BlockList:         "words",
			BlockListBehavior: stream.ModBehaviourFlag,
		},
		Commands: []*stream.Command{{Name: "giphy"}},
		Grants:   map[string][]string{"user": {"read-channel"}},
	}
	var data channelTypeResourceModel
	diags := mapChannelTypeToModel(context.Background(), ct, &data)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if data.Name.ValueString() != "team" {
		t.Errorf("Name = %q", data.Name.ValueString())
	}
	if !data.TypingEvents.ValueBool() || !data.Reactions.ValueBool() {
		t.Error("bool fields not mapped")
	}
	if data.MaxMessageLength.ValueInt64() != 256 {
		t.Errorf("MaxMessageLength = %d", data.MaxMessageLength.ValueInt64())
	}
	if data.Automod.ValueString() != "simple" {
		t.Errorf("Automod = %q", data.Automod.ValueString())
	}
	if data.AutomodBehavior.ValueString() != "block" {
		t.Errorf("AutomodBehavior = %q", data.AutomodBehavior.ValueString())
	}
	if data.BlocklistBehavior.ValueString() != "flag" {
		t.Errorf("BlocklistBehavior = %q", data.BlocklistBehavior.ValueString())
	}
	var cmds []string
	data.Commands.ElementsAs(context.Background(), &cmds, false)
	if len(cmds) != 1 || cmds[0] != "giphy" {
		t.Errorf("Commands = %v", cmds)
	}
	grants := map[string][]string{}
	data.Grants.ElementsAs(context.Background(), &grants, false)
	if g := grants["user"]; len(g) != 1 || g[0] != "read-channel" {
		t.Errorf("Grants = %v", grants)
	}
}

func TestMapChannelTypeToModel_NilErrors(t *testing.T) {
	t.Parallel()
	var data channelTypeResourceModel
	diags := mapChannelTypeToModel(context.Background(), nil, &data)
	if !diags.HasError() {
		t.Fatal("expected error for nil channel type")
	}
}
