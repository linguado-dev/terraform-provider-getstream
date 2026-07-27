package provider

import (
	"context"
	"fmt"

	stream "github.com/GetStream/stream-chat-go/v6"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the resource fully satisfies the framework interfaces.
var (
	_ resource.Resource                = &appSettingsResource{}
	_ resource.ResourceWithConfigure   = &appSettingsResource{}
	_ resource.ResourceWithImportState = &appSettingsResource{}
)

// appSettingsID is the fixed id for the app-settings singleton (there is exactly
// one settings object per app / per credential set).
const appSettingsID = "getstream-app-settings"

func NewAppSettingsResource() resource.Resource {
	return &appSettingsResource{}
}

type appSettingsResource struct {
	client *stream.Client
}

// appSettingsResourceModel maps the writable fields of the GetStream app settings
// object (GET/PUT /api/v2/app). Fields the API does not return on read (secrets)
// are noted below and kept config-authoritative.
type appSettingsResourceModel struct {
	Id types.String `tfsdk:"id"`

	// Event delivery.
	SqsURL      types.String `tfsdk:"sqs_url"`
	SqsKey      types.String `tfsdk:"sqs_key"`
	SqsSecret   types.String `tfsdk:"sqs_secret"` // write-only, not refreshed
	SnsTopicARN types.String `tfsdk:"sns_topic_arn"`
	SnsKey      types.String `tfsdk:"sns_key"`
	SnsSecret   types.String `tfsdk:"sns_secret"` // write-only, not refreshed

	// Webhooks / hooks.
	WebhookURL               types.String `tfsdk:"webhook_url"`
	WebhookEvents            types.List   `tfsdk:"webhook_events"`
	BeforeMessageSendHookURL types.String `tfsdk:"before_message_send_hook_url"`
	CustomActionHandlerURL   types.String `tfsdk:"custom_action_handler_url"`

	// Uploads.
	FileUploadConfig  types.Object `tfsdk:"file_upload_config"`
	ImageUploadConfig types.Object `tfsdk:"image_upload_config"`

	// Moderation.
	ImageModerationEnabled types.Bool `tfsdk:"image_moderation_enabled"`
	ImageModerationLabels  types.List `tfsdk:"image_moderation_labels"`

	// Permissions / behavior toggles.
	DisableAuth            types.Bool   `tfsdk:"disable_auth"`
	DisablePermissions     types.Bool   `tfsdk:"disable_permissions"`
	MultiTenantEnabled     types.Bool   `tfsdk:"multi_tenant_enabled"`
	AsyncURLEnrichEnabled  types.Bool   `tfsdk:"async_url_enrich_enabled"`
	AutoTranslationEnabled types.Bool   `tfsdk:"auto_translation_enabled"`
	RemindersInterval      types.Int64  `tfsdk:"reminders_interval"`
	ChannelHideMembersOnly types.Bool   `tfsdk:"channel_hide_members_only"`
	PermissionVersion      types.String `tfsdk:"permission_version"`
}

// uploadConfigAttrTypes is the attribute-type map for the file/image upload
// config nested object.
var uploadConfigAttrTypes = map[string]attr.Type{
	"allowed_file_extensions": types.ListType{ElemType: types.StringType},
	"blocked_file_extensions": types.ListType{ElemType: types.StringType},
	"allowed_mime_types":      types.ListType{ElemType: types.StringType},
	"blocked_mime_types":      types.ListType{ElemType: types.StringType},
}

func (r *appSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app_settings"
}

func (r *appSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	// Fields are Optional (not Computed): this resource manages the subset of app
	// settings you declare. Unset attributes stay null and are neither written nor
	// refreshed, so they don't produce spurious drift against server-side defaults.
	optString := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: desc,
		}
	}

	uploadCfg := func(desc string) schema.SingleNestedAttribute {
		return schema.SingleNestedAttribute{
			Optional:            true,
			MarkdownDescription: desc,
			Attributes: map[string]schema.Attribute{
				"allowed_file_extensions": schema.ListAttribute{ElementType: types.StringType, Optional: true},
				"blocked_file_extensions": schema.ListAttribute{ElementType: types.StringType, Optional: true},
				"allowed_mime_types":      schema.ListAttribute{ElementType: types.StringType, Optional: true},
				"blocked_mime_types":      schema.ListAttribute{ElementType: types.StringType, Optional: true},
			},
		}
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage the GetStream.io application-level settings singleton (GET/PUT `/api/v2/app`): " +
			"event delivery (SQS/SNS), webhooks, uploads, moderation, and behavior toggles. There is exactly one " +
			"of these per app. Note: secret fields (`sqs_secret`, `sns_secret`) are write-only — GetStream does not " +
			"return them, so they are kept from configuration and not refreshed for drift detection.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier (fixed; app settings are a singleton).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},

			"sqs_url":       optString("URL of the SQS queue GetStream publishes events to."),
			"sqs_key":       optString("Access key for the SQS queue."),
			"sqs_secret":    schema.StringAttribute{Optional: true, Sensitive: true, MarkdownDescription: "Secret key for the SQS queue. Write-only; not returned by the API."},
			"sns_topic_arn": optString("ARN of the SNS topic GetStream publishes events to."),
			"sns_key":       optString("Access key for the SNS topic."),
			"sns_secret":    schema.StringAttribute{Optional: true, Sensitive: true, MarkdownDescription: "Secret key for the SNS topic. Write-only; not returned by the API."},

			"webhook_url": optString("URL GetStream posts webhook events to."),
			"webhook_events": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Event types delivered to the webhook URL.",
			},
			"before_message_send_hook_url": optString("URL for the before-message-send hook."),
			"custom_action_handler_url":    optString("URL for the custom action handler."),

			"file_upload_config":  uploadCfg("Allowed/blocked file extensions and MIME types for file uploads."),
			"image_upload_config": uploadCfg("Allowed/blocked file extensions and MIME types for image uploads."),

			"image_moderation_enabled": schema.BoolAttribute{Optional: true, MarkdownDescription: "Whether AWS Rekognition image moderation is enabled."},
			"image_moderation_labels": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "Moderation labels to block.",
			},

			"disable_auth":              schema.BoolAttribute{Optional: true, MarkdownDescription: "Disable auth checks (development only)."},
			"disable_permissions":       schema.BoolAttribute{Optional: true, MarkdownDescription: "Disable permission checks (development only)."},
			"multi_tenant_enabled":      schema.BoolAttribute{Optional: true, MarkdownDescription: "Enable multi-tenant (teams) mode."},
			"async_url_enrich_enabled":  schema.BoolAttribute{Optional: true, MarkdownDescription: "Enable asynchronous URL enrichment."},
			"auto_translation_enabled":  schema.BoolAttribute{Optional: true, MarkdownDescription: "Enable automatic message translation."},
			"reminders_interval":        schema.Int64Attribute{Optional: true, MarkdownDescription: "Reminders interval in seconds."},
			"channel_hide_members_only": schema.BoolAttribute{Optional: true, MarkdownDescription: "Hide channels for members only on deletion."},
			"permission_version":        optString("Permission system version (e.g. \"v1\" or \"v2\")."),
		},
	}
}

func (r *appSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *appSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data appSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *appSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data appSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// apply performs the read-modify-write against the app settings singleton. App
// settings are a singleton that always exists, so Create and Update share this path.
// It sets data.Id and, for fields the API echoes back, refreshes them from the
// post-write response — but keeps write-only secrets from the plan (the API never
// returns them, so re-reading would blank them and break drift detection).
func (r *appSettingsResource) apply(ctx context.Context, data *appSettingsResourceModel, diags *diag.Diagnostics) {
	settings := &stream.AppSettings{}
	diags.Append(modelToAppSettings(ctx, *data, settings)...)
	if diags.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating app settings on GetStream.io")
	if _, err := r.client.UpdateAppSettings(ctx, settings); err != nil {
		diags.AddError("Error updating app settings", err.Error())
		return
	}
	data.Id = types.StringValue(appSettingsID)
	// The plan is the authoritative post-apply state (GetStream's read-after-write is
	// eventually consistent, and secrets are never returned); the plan holds all
	// known/config values and computed values carried via UseStateForUnknown.
}

func (r *appSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data appSettingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	app, err := r.client.GetAppSettings(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading app settings", err.Error())
		return
	}
	resp.Diagnostics.Append(mapAppSettingsToModel(ctx, app.App, &data)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *appSettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// App settings are a singleton that always exists; there is nothing to delete
	// upstream. Removing the resource from state simply stops Terraform managing it.
	tflog.Debug(ctx, "Removing getstream_app_settings from state (settings singleton is not deleted upstream)")
}

func (r *appSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), appSettingsID)...)
}

// ---------------------------------------------------------------------------
// model <-> SDK mapping
// ---------------------------------------------------------------------------

func boolPtr(b bool) *bool      { return &b }
func strPtrAS(s string) *string { return &s }

// modelToAppSettings copies known (config-set) attributes from the model onto a
// *stream.AppSettings for the write. Unset (null/unknown) attributes are left off
// so GetStream keeps its current/default values.
func modelToAppSettings(ctx context.Context, data appSettingsResourceModel, s *stream.AppSettings) diag.Diagnostics {
	var diags diag.Diagnostics

	if v, ok := knownString(data.SqsURL); ok {
		s.SqsURL = strPtrAS(v)
	}
	if v, ok := knownString(data.SqsKey); ok {
		s.SqsKey = strPtrAS(v)
	}
	if v, ok := knownString(data.SqsSecret); ok {
		s.SqsSecret = strPtrAS(v)
	}
	if v, ok := knownString(data.SnsTopicARN); ok {
		s.SnsTopicArn = strPtrAS(v)
	}
	if v, ok := knownString(data.SnsKey); ok {
		s.SnsKey = strPtrAS(v)
	}
	if v, ok := knownString(data.SnsSecret); ok {
		s.SnsSecret = strPtrAS(v)
	}

	if v, ok := knownString(data.WebhookURL); ok {
		s.WebhookURL = strPtrAS(v)
	}
	if !data.WebhookEvents.IsNull() && !data.WebhookEvents.IsUnknown() {
		var events []string
		diags.Append(data.WebhookEvents.ElementsAs(ctx, &events, false)...)
		s.WebhookEvents = events
	}
	if v, ok := knownString(data.BeforeMessageSendHookURL); ok {
		s.BeforeMessageSendHookURL = strPtrAS(v)
	}
	if v, ok := knownString(data.CustomActionHandlerURL); ok {
		s.CustomActionHandlerURL = strPtrAS(v)
	}

	if cfg, d, ok := uploadConfigFromObject(ctx, data.FileUploadConfig); ok {
		diags.Append(d...)
		s.FileUploadConfig = cfg
	}
	if cfg, d, ok := uploadConfigFromObject(ctx, data.ImageUploadConfig); ok {
		diags.Append(d...)
		s.ImageUploadConfig = cfg
	}

	if !data.ImageModerationEnabled.IsNull() && !data.ImageModerationEnabled.IsUnknown() {
		s.ImageModerationEnabled = boolPtr(data.ImageModerationEnabled.ValueBool())
	}
	if !data.ImageModerationLabels.IsNull() && !data.ImageModerationLabels.IsUnknown() {
		var labels []string
		diags.Append(data.ImageModerationLabels.ElementsAs(ctx, &labels, false)...)
		s.ImageModerationLabels = labels
	}

	if !data.DisableAuth.IsNull() && !data.DisableAuth.IsUnknown() {
		s.DisableAuth = boolPtr(data.DisableAuth.ValueBool())
	}
	if !data.DisablePermissions.IsNull() && !data.DisablePermissions.IsUnknown() {
		s.DisablePermissions = boolPtr(data.DisablePermissions.ValueBool())
	}
	if !data.MultiTenantEnabled.IsNull() && !data.MultiTenantEnabled.IsUnknown() {
		s.MultiTenantEnabled = boolPtr(data.MultiTenantEnabled.ValueBool())
	}
	if !data.AsyncURLEnrichEnabled.IsNull() && !data.AsyncURLEnrichEnabled.IsUnknown() {
		s.AsyncURLEnrichEnabled = boolPtr(data.AsyncURLEnrichEnabled.ValueBool())
	}
	if !data.AutoTranslationEnabled.IsNull() && !data.AutoTranslationEnabled.IsUnknown() {
		s.AutoTranslationEnabled = boolPtr(data.AutoTranslationEnabled.ValueBool())
	}
	if !data.RemindersInterval.IsNull() && !data.RemindersInterval.IsUnknown() {
		s.RemindersInterval = int(data.RemindersInterval.ValueInt64())
	}
	if !data.ChannelHideMembersOnly.IsNull() && !data.ChannelHideMembersOnly.IsUnknown() {
		s.ChannelHideMembersOnly = boolPtr(data.ChannelHideMembersOnly.ValueBool())
	}
	if v, ok := knownString(data.PermissionVersion); ok {
		s.PermissionVersion = strPtrAS(v)
	}

	return diags
}

// uploadConfigFromObject converts a nested upload-config object to the SDK type.
// The third return is false when the object is null/unknown (nothing to set).
func uploadConfigFromObject(ctx context.Context, obj types.Object) (*stream.FileUploadConfig, diag.Diagnostics, bool) {
	var diags diag.Diagnostics
	if obj.IsNull() || obj.IsUnknown() {
		return nil, diags, false
	}
	var m struct {
		AllowedFileExtensions []string `tfsdk:"allowed_file_extensions"`
		BlockedFileExtensions []string `tfsdk:"blocked_file_extensions"`
		AllowedMimeTypes      []string `tfsdk:"allowed_mime_types"`
		BlockedMimeTypes      []string `tfsdk:"blocked_mime_types"`
	}
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	return &stream.FileUploadConfig{
		AllowedFileExtensions: m.AllowedFileExtensions,
		BlockedFileExtensions: m.BlockedFileExtensions,
		AllowedMimeTypes:      m.AllowedMimeTypes,
		BlockedMimeTypes:      m.BlockedMimeTypes,
	}, diags, true
}

// mapAppSettingsToModel refreshes the model from an app settings read. Because this
// resource manages only the subset of settings the user declares, a field is
// refreshed ONLY if it is already set (non-null) in state — otherwise reading back
// server-side values for unmanaged fields would create perpetual spurious drift.
// Write-only secret fields (sqs_secret, sns_secret) are never touched: the API does
// not return them, so the value from configuration is preserved.
func mapAppSettingsToModel(ctx context.Context, s *stream.AppSettings, data *appSettingsResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if s == nil {
		diags.AddError("Empty app settings response", "GetStream.io returned no app body.")
		return diags
	}
	data.Id = types.StringValue(appSettingsID)

	// refreshString overwrites dst from src only if dst is currently managed (non-null).
	refreshString := func(dst *types.String, src *string) {
		if !dst.IsNull() {
			*dst = stringFromPtr(src)
		}
	}
	refreshBool := func(dst *types.Bool, src *bool) {
		if !dst.IsNull() {
			*dst = boolFromPtr(src)
		}
	}

	refreshString(&data.SqsURL, s.SqsURL)
	refreshString(&data.SqsKey, s.SqsKey)
	// SqsSecret intentionally left as-is (write-only).
	refreshString(&data.SnsTopicARN, s.SnsTopicArn)
	refreshString(&data.SnsKey, s.SnsKey)
	// SnsSecret intentionally left as-is (write-only).

	refreshString(&data.WebhookURL, s.WebhookURL)
	if !data.WebhookEvents.IsNull() {
		events, d := types.ListValueFrom(ctx, types.StringType, s.WebhookEvents)
		diags.Append(d...)
		data.WebhookEvents = events
	}
	refreshString(&data.BeforeMessageSendHookURL, s.BeforeMessageSendHookURL)
	refreshString(&data.CustomActionHandlerURL, s.CustomActionHandlerURL)

	if !data.FileUploadConfig.IsNull() {
		fu, d := uploadConfigToObject(ctx, s.FileUploadConfig)
		diags.Append(d...)
		data.FileUploadConfig = fu
	}
	if !data.ImageUploadConfig.IsNull() {
		iu, d := uploadConfigToObject(ctx, s.ImageUploadConfig)
		diags.Append(d...)
		data.ImageUploadConfig = iu
	}

	refreshBool(&data.ImageModerationEnabled, s.ImageModerationEnabled)
	if !data.ImageModerationLabels.IsNull() {
		labels, d := types.ListValueFrom(ctx, types.StringType, s.ImageModerationLabels)
		diags.Append(d...)
		data.ImageModerationLabels = labels
	}

	refreshBool(&data.DisableAuth, s.DisableAuth)
	refreshBool(&data.DisablePermissions, s.DisablePermissions)
	refreshBool(&data.MultiTenantEnabled, s.MultiTenantEnabled)
	refreshBool(&data.AsyncURLEnrichEnabled, s.AsyncURLEnrichEnabled)
	refreshBool(&data.AutoTranslationEnabled, s.AutoTranslationEnabled)
	if !data.RemindersInterval.IsNull() {
		data.RemindersInterval = types.Int64Value(int64(s.RemindersInterval))
	}
	refreshBool(&data.ChannelHideMembersOnly, s.ChannelHideMembersOnly)
	refreshString(&data.PermissionVersion, s.PermissionVersion)

	return diags
}

func uploadConfigToObject(ctx context.Context, c *stream.FileUploadConfig) (types.Object, diag.Diagnostics) {
	if c == nil {
		return types.ObjectNull(uploadConfigAttrTypes), nil
	}
	return types.ObjectValueFrom(ctx, uploadConfigAttrTypes, struct {
		AllowedFileExtensions []string `tfsdk:"allowed_file_extensions"`
		BlockedFileExtensions []string `tfsdk:"blocked_file_extensions"`
		AllowedMimeTypes      []string `tfsdk:"allowed_mime_types"`
		BlockedMimeTypes      []string `tfsdk:"blocked_mime_types"`
	}{c.AllowedFileExtensions, c.BlockedFileExtensions, c.AllowedMimeTypes, c.BlockedMimeTypes})
}

func stringFromPtr(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

func boolFromPtr(p *bool) types.Bool {
	if p == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*p)
}
