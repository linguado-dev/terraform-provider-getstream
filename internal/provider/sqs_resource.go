package provider

import (
	"context"
	"fmt"
	"net/url"

	stream "github.com/GetStream/stream-chat-go/v6"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the resource fully satisfies the framework interfaces.
var (
	_ resource.Resource                = &sqsResource{}
	_ resource.ResourceWithConfigure   = &sqsResource{}
	_ resource.ResourceWithImportState = &sqsResource{}
)

// sqsResourceID is the fixed id for the singleton SQS configuration on an app.
const sqsResourceID = "getstream-sqs-1"

func NewSqsResource() resource.Resource {
	return &sqsResource{}
}

type sqsResource struct {
	client *stream.Client
}

type sqsResourceModel struct {
	Id           types.String `tfsdk:"id"`
	SqsUrl       types.String `tfsdk:"sqs_url"`
	SqsAccessKey types.String `tfsdk:"sqs_access_key"`
	SqsSecretKey types.String `tfsdk:"sqs_secret_key"`
}

func (r *sqsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sqs"
}

func (r *sqsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Configure the SQS queue that GetStream.io publishes application events to.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"sqs_url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "URL of the SQS queue to send messages to.",
				Validators: []validator.String{
					urlValidator{},
				},
			},
			"sqs_access_key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Access key with privileges to send messages on the SQS queue.",
			},
			"sqs_secret_key": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Secret key with privileges to send messages on the SQS queue.",
			},
		},
	}
}

func (r *sqsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *sqsResource) applySettings(ctx context.Context, data sqsResourceModel) error {
	settings := &stream.AppSettings{
		SqsURL:    stringPtr(data.SqsUrl.ValueString()),
		SqsKey:    stringPtr(data.SqsAccessKey.ValueString()),
		SqsSecret: stringPtr(data.SqsSecretKey.ValueString()),
	}
	_, err := r.client.UpdateAppSettings(ctx, settings)
	return err
}

func (r *sqsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data sqsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating the SQS link on GetStream.io")
	if err := r.applySettings(ctx, data); err != nil {
		resp.Diagnostics.AddError("Error configuring the SQS link", err.Error())
		return
	}

	data.Id = types.StringValue(sqsResourceID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *sqsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data sqsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// GetStream.io does not return the SQS secret, so the credentials cannot be
	// refreshed from the API; the state is kept as-is.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *sqsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data sqsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating the SQS link on GetStream.io")
	if err := r.applySettings(ctx, data); err != nil {
		resp.Diagnostics.AddError("Error updating the SQS link", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *sqsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data sqsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Clear the SQS configuration on the app.
	tflog.Debug(ctx, "Deleting the SQS link on GetStream.io")
	empty := sqsResourceModel{
		SqsUrl:       types.StringValue(""),
		SqsAccessKey: types.StringValue(""),
		SqsSecretKey: types.StringValue(""),
	}
	if err := r.applySettings(ctx, empty); err != nil {
		resp.Diagnostics.AddError("Error deleting the SQS link", err.Error())
		return
	}
}

func (r *sqsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func stringPtr(s string) *string { return &s }

// urlValidator validates that a string attribute is a well-formed absolute URL.
type urlValidator struct{}

func (urlValidator) Description(context.Context) string {
	return "value must be a valid absolute URL"
}

func (urlValidator) MarkdownDescription(context.Context) string {
	return "value must be a valid absolute URL"
}

func (urlValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	u, err := url.Parse(req.ConfigValue.ValueString())
	if err != nil || u.Scheme == "" || u.Host == "" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid URL",
			fmt.Sprintf("Attribute %s must be a valid absolute URL, got: %q", req.Path, req.ConfigValue.ValueString()),
		)
	}
}
