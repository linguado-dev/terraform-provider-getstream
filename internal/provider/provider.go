package provider

import (
	"context"
	"fmt"
	"os"
	"strings"

	stream "github.com/GetStream/stream-chat-go/v6"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Environment variables used as fallbacks for the provider credentials when the
// corresponding configuration attributes are not set. GetStream's public docs use
// the STREAM_API_* names while its Go SDK (NewClientFromEnvVars) reads STREAM_KEY /
// STREAM_SECRET, so both are accepted, docs name first.
var (
	envAPIKeyNames    = []string{"STREAM_API_KEY", "STREAM_KEY"}
	envAPISecretNames = []string{"STREAM_API_SECRET", "STREAM_SECRET"}
)

const (
	envAppName = "STREAM_APP_NAME"
	envAppID   = "STREAM_APP_ID"
)

// Ensure the provider fully satisfies the framework interfaces.
var _ provider.Provider = &getstreamProvider{}

// getstreamProvider implements provider.Provider.
type getstreamProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and run locally, and "test" when running acceptance
	// testing.
	version string
}

// providerData holds the configured GetStream.io client plus the identity of the
// app it is bound to, and is passed to resources and data sources via their
// Configure methods.
type providerData struct {
	client *stream.Client
	// appName / appOrg are read back from the app during Configure so data
	// sources can surface them without another API round-trip.
	appName string
	appOrg  string
}

// providerModel maps the provider configuration schema.
type providerModel struct {
	ApiKey    types.String `tfsdk:"api_key"`
	ApiSecret types.String `tfsdk:"api_secret"`
	AppName   types.String `tfsdk:"app_name"`
	AppID     types.String `tfsdk:"app_id"`
}

func (p *getstreamProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "getstream"
	resp.Version = p.version
}

func (p *getstreamProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage GetStream.io application configuration.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				MarkdownDescription: "GetStream.io API key. May also be set via the `" + strings.Join(envAPIKeyNames, "` or `") + "` environment variable.",
				Optional:            true,
			},
			"api_secret": schema.StringAttribute{
				MarkdownDescription: "GetStream.io API secret. May also be set via the `" + strings.Join(envAPISecretNames, "` or `") + "` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"app_name": schema.StringAttribute{
				MarkdownDescription: "Expected name of the GetStream.io app these credentials target. When set, the provider verifies the app the credentials resolve to has this name and fails otherwise. This guards against pointing an environment's Terraform at the wrong app (e.g. a prod key in a non-prod config). May also be set via the `" + envAppName + "` environment variable.",
				Optional:            true,
			},
			"app_id": schema.StringAttribute{
				MarkdownDescription: "Numeric GetStream.io app ID, accepted for documentation and cross-reference in configuration. The app-scoped API does not expose the app ID, so this value is not verified against the credentials and is not persisted to state. Prefer `app_name` for the wrong-app guard. May also be set via the `" + envAppID + "` environment variable.",
				Optional:            true,
			},
		},
	}
}

func (p *getstreamProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := firstNonEmpty(data.ApiKey, firstEnv(envAPIKeyNames))
	apiSecret := firstNonEmpty(data.ApiSecret, firstEnv(envAPISecretNames))

	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing GetStream.io API key",
			fmt.Sprintf("Set the api_key attribute or one of these environment variables: %s.", strings.Join(envAPIKeyNames, ", ")),
		)
	}
	if apiSecret == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_secret"),
			"Missing GetStream.io API secret",
			fmt.Sprintf("Set the api_secret attribute or one of these environment variables: %s.", strings.Join(envAPISecretNames, ", ")),
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating GetStream.io client")
	client, err := stream.NewClient(apiKey, apiSecret)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create GetStream.io client", err.Error())
		return
	}

	// A single GetAppSettings call both validates the credentials and yields the
	// app identity used for the app_name guard below.
	app, err := client.GetAppSettings(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Invalid GetStream.io credentials", err.Error())
		return
	}

	pd := &providerData{client: client}
	if app.App != nil {
		pd.appName = app.App.Name
		pd.appOrg = app.App.OrganizationName
	}

	// Wrong-app guard: if the operator declared an expected app_name (via config
	// or the STREAM_APP_NAME env var), the credentials must resolve to an app
	// with that name.
	if expected := firstNonEmpty(data.AppName, os.Getenv(envAppName)); expected != "" {
		if pd.appName != expected {
			resp.Diagnostics.AddAttributeError(
				path.Root("app_name"),
				"GetStream.io app name mismatch",
				fmt.Sprintf("The configured credentials resolve to app %q, but app_name is set to %q. "+
					"Check that this environment's api_key/api_secret target the intended app.", pd.appName, expected),
			)
			return
		}
	}

	resp.ResourceData = pd
	resp.DataSourceData = pd
}

func (p *getstreamProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewSqsResource,
		NewChannelTypeResource,
	}
}

func (p *getstreamProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewAppDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &getstreamProvider{version: version}
	}
}

// firstNonEmpty returns the string value of v if it is set (non-null, non-unknown,
// non-empty), otherwise the fallback.
func firstNonEmpty(v types.String, fallback string) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	return fallback
}

// firstEnv returns the value of the first set (non-empty) environment variable
// among names, or "" if none are set.
func firstEnv(names []string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}
