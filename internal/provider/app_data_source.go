package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the data source fully satisfies the framework interfaces.
var (
	_ datasource.DataSource              = &appDataSource{}
	_ datasource.DataSourceWithConfigure = &appDataSource{}
)

func NewAppDataSource() datasource.DataSource {
	return &appDataSource{}
}

type appDataSource struct {
	// providerData carries the app identity resolved once during provider
	// Configure, so the data source does not need another API round-trip.
	pd *providerData
}

type appDataSourceModel struct {
	Name         types.String `tfsdk:"name"`
	Organization types.String `tfsdk:"organization"`
}

func (d *appDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_app"
}

func (d *appDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the identity of the GetStream.io app the configured credentials resolve to. " +
			"Use this with a `precondition` to assert Terraform is pointed at the intended app. " +
			"Note: the app-scoped API does not expose the numeric app ID, only its name and organization.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Name of the app.",
			},
			"organization": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Organization the app belongs to.",
			},
		},
	}
}

func (d *appDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *providerData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	d.pd = pd
}

func (d *appDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// The app identity was resolved during provider Configure (which calls
	// GetAppSettings once); reuse it rather than making another API call.
	data := appDataSourceModel{
		Name:         types.StringValue(d.pd.appName),
		Organization: types.StringValue(d.pd.appOrg),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
