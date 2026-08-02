package provider

import (
	"context"
	"fmt"

	stream "github.com/GetStream/stream-chat-go/v6"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the resource fully satisfies the framework interfaces.
var (
	_ resource.Resource                = &commandResource{}
	_ resource.ResourceWithConfigure   = &commandResource{}
	_ resource.ResourceWithImportState = &commandResource{}
)

func NewCommandResource() resource.Resource {
	return &commandResource{}
}

type commandResource struct {
	client *stream.Client
}

// commandResourceModel maps the getstream_command schema onto stream.Command.
type commandResourceModel struct {
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Args        types.String `tfsdk:"args"`
	Set         types.String `tfsdk:"set"`
}

func (r *commandResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_command"
}

func (r *commandResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a GetStream.io custom command (a chat slash-command). " +
			"Commands are referenced by name from `getstream_channel_type.commands`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Unique name of the command (without the leading slash). This is the identifier; renaming forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Human-readable description of the command.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"args": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Argument hint shown to users, e.g. \"[@username]\".",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"set": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Command set the command belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *commandResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *commandResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data commandResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating command on GetStream.io", map[string]any{"name": data.Name.ValueString()})
	created, err := r.client.CreateCommand(ctx, commandFromModel(data))
	if err != nil {
		resp.Diagnostics.AddError("Error creating command", err.Error())
		return
	}

	mapCommandToModel(created.Command, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *commandResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data commandResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetCommand(ctx, data.Name.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading command", err.Error())
		return
	}

	mapCommandToModel(got.Command, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *commandResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data commandResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating command on GetStream.io", map[string]any{"name": data.Name.ValueString()})
	updated, err := r.client.UpdateCommand(ctx, data.Name.ValueString(), commandFromModel(data))
	if err != nil {
		resp.Diagnostics.AddError("Error updating command", err.Error())
		return
	}

	mapCommandToModel(updated.Command, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *commandResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data commandResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting command on GetStream.io", map[string]any{"name": data.Name.ValueString()})
	if _, err := r.client.DeleteCommand(ctx, data.Name.ValueString()); err != nil {
		if isNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting command", err.Error())
		return
	}
}

func (r *commandResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The import ID is the command name; Read hydrates the rest.
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

// commandFromModel builds a *stream.Command from the model for Create/Update.
// stream.Command's json tags have no omitempty, so unset optional fields are sent
// as empty strings (the API accepts an empty description/args/set as "no value").
// The Computed + UseStateForUnknown attributes then hold whatever the API echoes
// back, so state stays consistent.
func commandFromModel(data commandResourceModel) *stream.Command {
	cmd := &stream.Command{Name: data.Name.ValueString()}
	if s, ok := knownString(data.Description); ok {
		cmd.Description = s
	}
	if s, ok := knownString(data.Args); ok {
		cmd.Args = s
	}
	if s, ok := knownString(data.Set); ok {
		cmd.Set = s
	}
	return cmd
}

// mapCommandToModel hydrates the model from a command returned by the API.
func mapCommandToModel(cmd *stream.Command, data *commandResourceModel) {
	if cmd == nil {
		return
	}
	data.Name = types.StringValue(cmd.Name)
	data.Description = types.StringValue(cmd.Description)
	data.Args = types.StringValue(cmd.Args)
	data.Set = types.StringValue(cmd.Set)
}
