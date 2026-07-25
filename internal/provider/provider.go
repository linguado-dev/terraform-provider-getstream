package provider

import (
	"context"

	stream "github.com/GetStream/stream-chat-go/v6"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
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

// providerData holds the configured GetStream.io client and is passed to
// resources and data sources via their Configure methods.
type providerData struct {
	client *stream.Client
}

// providerModel maps the provider configuration schema.
type providerModel struct {
	ApiKey    types.String `tfsdk:"api_key"`
	ApiSecret types.String `tfsdk:"api_secret"`
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
				MarkdownDescription: "GetStream.io API key.",
				Required:            true,
			},
			"api_secret": schema.StringAttribute{
				MarkdownDescription: "GetStream.io API secret.",
				Required:            true,
				Sensitive:           true,
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

	tflog.Debug(ctx, "Creating GetStream.io client")
	client, err := stream.NewClient(data.ApiKey.ValueString(), data.ApiSecret.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create GetStream.io client", err.Error())
		return
	}
	if _, err := client.GetAppSettings(ctx); err != nil {
		resp.Diagnostics.AddError("Invalid GetStream.io credentials", err.Error())
		return
	}

	pd := &providerData{client: client}
	resp.ResourceData = pd
	resp.DataSourceData = pd
}

func (p *getstreamProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewSqsResource,
	}
}

func (p *getstreamProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &getstreamProvider{version: version}
	}
}
