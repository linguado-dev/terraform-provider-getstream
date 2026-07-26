package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	stream "github.com/GetStream/stream-chat-go/v6"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the resource fully satisfies the framework interfaces.
var (
	_ resource.Resource                = &channelTypeResource{}
	_ resource.ResourceWithConfigure   = &channelTypeResource{}
	_ resource.ResourceWithImportState = &channelTypeResource{}
)

func NewChannelTypeResource() resource.Resource {
	return &channelTypeResource{}
}

type channelTypeResource struct {
	client *stream.Client
}

// channelTypeResourceModel maps the getstream_channel_type schema. Fields that
// GetStream.io populates with server-side defaults are Optional+Computed so
// that omitting them in configuration does not produce a perpetual diff.
type channelTypeResourceModel struct {
	Name              types.String `tfsdk:"name"`
	TypingEvents      types.Bool   `tfsdk:"typing_events"`
	ReadEvents        types.Bool   `tfsdk:"read_events"`
	ConnectEvents     types.Bool   `tfsdk:"connect_events"`
	Search            types.Bool   `tfsdk:"search"`
	Reactions         types.Bool   `tfsdk:"reactions"`
	Reminders         types.Bool   `tfsdk:"reminders"`
	Replies           types.Bool   `tfsdk:"replies"`
	Mutes             types.Bool   `tfsdk:"mutes"`
	PushNotifications types.Bool   `tfsdk:"push_notifications"`
	Uploads           types.Bool   `tfsdk:"uploads"`
	URLEnrichment     types.Bool   `tfsdk:"url_enrichment"`
	CustomEvents      types.Bool   `tfsdk:"custom_events"`
	MessageRetention  types.String `tfsdk:"message_retention"`
	MaxMessageLength  types.Int64  `tfsdk:"max_message_length"`
	Automod           types.String `tfsdk:"automod"`
	AutomodBehavior   types.String `tfsdk:"automod_behavior"`
	Blocklist         types.String `tfsdk:"blocklist"`
	BlocklistBehavior types.String `tfsdk:"blocklist_behavior"`
	Commands          types.List   `tfsdk:"commands"`
	Grants            types.Map    `tfsdk:"grants"`
}

func (r *channelTypeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_channel_type"
}

func (r *channelTypeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	// boolAttr builds an Optional+Computed boolean whose value is preserved from
	// state when not set in configuration, avoiding perpetual diffs.
	boolAttr := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			MarkdownDescription: desc,
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.UseStateForUnknown(),
			},
		}
	}
	strAttr := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			MarkdownDescription: desc,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		}
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a GetStream.io channel type and its configuration.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unique name of the channel type. This is the identifier; renaming forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"typing_events":      boolAttr("Whether typing indicator events are sent."),
			"read_events":        boolAttr("Whether read receipts are tracked."),
			"connect_events":     boolAttr("Whether connect/disconnect events are sent."),
			"search":             boolAttr("Whether messages are indexed for search."),
			"reactions":          boolAttr("Whether message reactions are enabled."),
			"reminders":          boolAttr("Whether reminders are enabled."),
			"replies":            boolAttr("Whether threaded replies are enabled."),
			"mutes":              boolAttr("Whether users can mute channels."),
			"push_notifications": boolAttr("Whether push notifications are enabled."),
			"uploads":            boolAttr("Whether file uploads are enabled."),
			"url_enrichment":     boolAttr("Whether URLs are enriched with previews."),
			"custom_events":      boolAttr("Whether custom events are enabled."),
			"message_retention":  strAttr("Message retention: \"infinite\" or a numeric number of days."),
			"max_message_length": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Maximum length of a message in characters.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"automod":            strAttr("Automod mode: one of \"disabled\", \"simple\", or \"AI\"."),
			"automod_behavior":   strAttr("Automod behavior: one of \"flag\" or \"block\"."),
			"blocklist":          strAttr("Name of the blocklist applied to this channel type."),
			"blocklist_behavior": strAttr("Blocklist behavior: one of \"flag\" or \"block\"."),
			"commands": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Commands enabled on the channel type (e.g. [\"all\"]).",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"grants": schema.MapAttribute{
				ElementType:         types.ListType{ElemType: types.StringType},
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Permission grants, keyed by role, each a list of grant strings.",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *channelTypeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *providerData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.client = pd.client
}

func (r *channelTypeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data channelTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ct := stream.NewChannelType(data.Name.ValueString())
	resp.Diagnostics.Append(applyModelToChannelType(ctx, data, ct)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating channel type on GetStream.io", map[string]any{"name": data.Name.ValueString()})
	created, err := r.client.CreateChannelType(ctx, ct)
	if err != nil {
		resp.Diagnostics.AddError("Error creating channel type", err.Error())
		return
	}

	resp.Diagnostics.Append(mapChannelTypeToModel(ctx, created.ChannelType, &data)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *channelTypeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data channelTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetChannelType(ctx, data.Name.ValueString())
	if err != nil {
		if isNotFound(err) {
			// The channel type was deleted out-of-band; drop it from state so
			// Terraform plans to recreate it.
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading channel type", err.Error())
		return
	}

	resp.Diagnostics.Append(mapChannelTypeToModel(ctx, got.ChannelType, &data)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *channelTypeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data channelTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	options, diags := updateOptions(ctx, data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating channel type on GetStream.io", map[string]any{"name": data.Name.ValueString()})
	if _, err := r.client.UpdateChannelType(ctx, data.Name.ValueString(), options); err != nil {
		resp.Diagnostics.AddError("Error updating channel type", err.Error())
		return
	}

	// UpdateChannelType returns no channel type body, so refresh from the API to
	// capture server-computed values.
	got, err := r.client.GetChannelType(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading channel type after update", err.Error())
		return
	}
	resp.Diagnostics.Append(mapChannelTypeToModel(ctx, got.ChannelType, &data)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *channelTypeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data channelTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting channel type on GetStream.io", map[string]any{"name": data.Name.ValueString()})
	if _, err := r.client.DeleteChannelType(ctx, data.Name.ValueString()); err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting channel type", err.Error())
		return
	}
}

func (r *channelTypeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The import ID is the channel type name; Read hydrates the rest.
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

// applyModelToChannelType copies the configured attributes onto a *stream.ChannelType
// for Create. Only known (config-set) values are applied; unset Optional+Computed
// attributes keep the SDK defaults from stream.NewChannelType so that GetStream.io
// fills them server-side.
func applyModelToChannelType(ctx context.Context, data channelTypeResourceModel, ct *stream.ChannelType) diag.Diagnostics {
	var diags diag.Diagnostics

	applyBool := func(v types.Bool, dst *bool) {
		if !v.IsNull() && !v.IsUnknown() {
			*dst = v.ValueBool()
		}
	}
	applyBool(data.TypingEvents, &ct.TypingEvents)
	applyBool(data.ReadEvents, &ct.ReadEvents)
	applyBool(data.ConnectEvents, &ct.ConnectEvents)
	applyBool(data.Search, &ct.Search)
	applyBool(data.Reactions, &ct.Reactions)
	applyBool(data.Reminders, &ct.Reminders)
	applyBool(data.Replies, &ct.Replies)
	applyBool(data.Mutes, &ct.Mutes)
	applyBool(data.PushNotifications, &ct.PushNotifications)
	applyBool(data.Uploads, &ct.Uploads)
	applyBool(data.URLEnrichment, &ct.URLEnrichment)
	applyBool(data.CustomEvents, &ct.CustomEvents)

	if s, ok := knownString(data.MessageRetention); ok {
		ct.MessageRetention = s
	}
	if !data.MaxMessageLength.IsNull() && !data.MaxMessageLength.IsUnknown() {
		ct.MaxMessageLength = int(data.MaxMessageLength.ValueInt64())
	}
	if s, ok := knownString(data.Blocklist); ok {
		ct.BlockList = s
	}

	// automod / behavior fields use unexported SDK types, so map the validated
	// string to the exported constants. Validation is shared with Update via
	// validateAutomod/validateBehavior so both paths reject bad input identically.
	if s, ok := knownString(data.Automod); ok {
		if d := validateAutomod(s); d != nil {
			diags.Append(d)
		} else {
			switch s {
			case string(stream.AutoModDisabled):
				ct.Automod = stream.AutoModDisabled
			case string(stream.AutoModSimple):
				ct.Automod = stream.AutoModSimple
			case string(stream.AutoModAI):
				ct.Automod = stream.AutoModAI
			}
		}
	}
	if s, ok := knownString(data.AutomodBehavior); ok {
		if d := validateBehavior(path.Root("automod_behavior"), s); d != nil {
			diags.Append(d)
		} else if s == string(stream.ModBehaviourBlock) {
			ct.ModBehavior = stream.ModBehaviourBlock
		} else {
			ct.ModBehavior = stream.ModBehaviourFlag
		}
	}
	if s, ok := knownString(data.BlocklistBehavior); ok {
		if d := validateBehavior(path.Root("blocklist_behavior"), s); d != nil {
			diags.Append(d)
		} else if s == string(stream.ModBehaviourBlock) {
			ct.BlockListBehavior = stream.ModBehaviourBlock
		} else {
			ct.BlockListBehavior = stream.ModBehaviourFlag
		}
	}

	if !data.Commands.IsNull() && !data.Commands.IsUnknown() {
		var names []string
		diags.Append(data.Commands.ElementsAs(ctx, &names, false)...)
		ct.Commands = nil
		for _, name := range names {
			ct.Commands = append(ct.Commands, &stream.Command{Name: name})
		}
	}
	if !data.Grants.IsNull() && !data.Grants.IsUnknown() {
		grants := map[string][]string{}
		diags.Append(data.Grants.ElementsAs(ctx, &grants, false)...)
		ct.Grants = grants
	}

	return diags
}

// updateOptions builds the options map for UpdateChannelType from all known
// attributes on the plan.
func updateOptions(ctx context.Context, data channelTypeResourceModel) (map[string]interface{}, diag.Diagnostics) {
	var diags diag.Diagnostics
	options := map[string]interface{}{}

	putBool := func(key string, v types.Bool) {
		if !v.IsNull() && !v.IsUnknown() {
			options[key] = v.ValueBool()
		}
	}
	putBool("typing_events", data.TypingEvents)
	putBool("read_events", data.ReadEvents)
	putBool("connect_events", data.ConnectEvents)
	putBool("search", data.Search)
	putBool("reactions", data.Reactions)
	putBool("reminders", data.Reminders)
	putBool("replies", data.Replies)
	putBool("mutes", data.Mutes)
	putBool("push_notifications", data.PushNotifications)
	putBool("uploads", data.Uploads)
	putBool("url_enrichment", data.URLEnrichment)
	putBool("custom_events", data.CustomEvents)

	if s, ok := knownString(data.MessageRetention); ok {
		options["message_retention"] = s
	}
	if !data.MaxMessageLength.IsNull() && !data.MaxMessageLength.IsUnknown() {
		options["max_message_length"] = data.MaxMessageLength.ValueInt64()
	}
	if s, ok := knownString(data.Automod); ok {
		if d := validateAutomod(s); d != nil {
			diags.Append(d)
		} else {
			options["automod"] = s
		}
	}
	if s, ok := knownString(data.AutomodBehavior); ok {
		if d := validateBehavior(path.Root("automod_behavior"), s); d != nil {
			diags.Append(d)
		} else {
			options["automod_behavior"] = s
		}
	}
	if s, ok := knownString(data.Blocklist); ok {
		options["blocklist"] = s
	}
	if s, ok := knownString(data.BlocklistBehavior); ok {
		if d := validateBehavior(path.Root("blocklist_behavior"), s); d != nil {
			diags.Append(d)
		} else {
			options["blocklist_behavior"] = s
		}
	}
	if !data.Commands.IsNull() && !data.Commands.IsUnknown() {
		var names []string
		diags.Append(data.Commands.ElementsAs(ctx, &names, false)...)
		options["commands"] = names
	}
	if !data.Grants.IsNull() && !data.Grants.IsUnknown() {
		grants := map[string][]string{}
		diags.Append(data.Grants.ElementsAs(ctx, &grants, false)...)
		options["grants"] = grants
	}

	return options, diags
}

// mapChannelTypeToModel hydrates the Terraform model from a channel type returned
// by the API.
func mapChannelTypeToModel(ctx context.Context, ct *stream.ChannelType, data *channelTypeResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if ct == nil {
		diags.AddError("Empty channel type response", "GetStream.io returned no channel type body.")
		return diags
	}

	data.Name = types.StringValue(ct.Name)
	data.TypingEvents = types.BoolValue(ct.TypingEvents)
	data.ReadEvents = types.BoolValue(ct.ReadEvents)
	data.ConnectEvents = types.BoolValue(ct.ConnectEvents)
	data.Search = types.BoolValue(ct.Search)
	data.Reactions = types.BoolValue(ct.Reactions)
	data.Reminders = types.BoolValue(ct.Reminders)
	data.Replies = types.BoolValue(ct.Replies)
	data.Mutes = types.BoolValue(ct.Mutes)
	data.PushNotifications = types.BoolValue(ct.PushNotifications)
	data.Uploads = types.BoolValue(ct.Uploads)
	data.URLEnrichment = types.BoolValue(ct.URLEnrichment)
	data.CustomEvents = types.BoolValue(ct.CustomEvents)
	data.MessageRetention = types.StringValue(ct.MessageRetention)
	data.MaxMessageLength = types.Int64Value(int64(ct.MaxMessageLength))
	data.Automod = types.StringValue(string(ct.Automod))
	data.AutomodBehavior = types.StringValue(string(ct.ModBehavior))
	data.Blocklist = types.StringValue(ct.BlockList)
	data.BlocklistBehavior = types.StringValue(string(ct.BlockListBehavior))

	names := make([]string, 0, len(ct.Commands))
	for _, cmd := range ct.Commands {
		if cmd != nil {
			names = append(names, cmd.Name)
		}
	}
	commands, d := types.ListValueFrom(ctx, types.StringType, names)
	diags.Append(d...)
	data.Commands = commands

	grants, d := types.MapValueFrom(ctx, types.ListType{ElemType: types.StringType}, ct.Grants)
	diags.Append(d...)
	data.Grants = grants

	return diags
}

// invalidBehaviorDiag returns a diagnostic for an unrecognized automod/blocklist
// behavior value.
func invalidBehaviorDiag(attr path.Path, s string) diag.Diagnostic {
	return diag.NewAttributeErrorDiagnostic(attr, "Invalid behavior value",
		fmt.Sprintf("value must be one of %q or %q, got: %q",
			stream.ModBehaviourFlag, stream.ModBehaviourBlock, s))
}

// validateAutomod returns a diagnostic if s is not a recognized automod value,
// or nil if it is valid. Used by both Create and Update so the two paths reject
// bad input identically instead of one deferring to an API error.
func validateAutomod(s string) diag.Diagnostic {
	switch s {
	case string(stream.AutoModDisabled), string(stream.AutoModSimple), string(stream.AutoModAI):
		return nil
	default:
		return diag.NewAttributeErrorDiagnostic(path.Root("automod"), "Invalid automod value",
			fmt.Sprintf("automod must be one of %q, %q, or %q, got: %q",
				stream.AutoModDisabled, stream.AutoModSimple, stream.AutoModAI, s))
	}
}

// validateBehavior returns a diagnostic if s is not a recognized flag/block
// behavior value, or nil if it is valid.
func validateBehavior(attr path.Path, s string) diag.Diagnostic {
	switch s {
	case string(stream.ModBehaviourFlag), string(stream.ModBehaviourBlock):
		return nil
	default:
		return invalidBehaviorDiag(attr, s)
	}
}

func knownString(v types.String) (string, bool) {
	if v.IsNull() || v.IsUnknown() {
		return "", false
	}
	return v.ValueString(), true
}

func isNotFound(err error) bool {
	var apiErr stream.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound
	}
	return false
}
