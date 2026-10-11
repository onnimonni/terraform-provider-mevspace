package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/onnimonni/terraform-provider-mevspace/internal/client"
)

var _ provider.Provider = &mevspaceProvider{}

type mevspaceProvider struct {
	version string
}

type providerModel struct {
	APIToken types.String `tfsdk:"api_token"`
	Endpoint types.String `tfsdk:"endpoint"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &mevspaceProvider{version: version}
	}
}

func (p *mevspaceProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "mevspace"
	resp.Version = p.version
}

func (p *mevspaceProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage MEVSpace (mevspace.com) dedicated servers and SSH keys.",
		Attributes: map[string]schema.Attribute{
			"api_token": schema.StringAttribute{
				Description: "MEVSpace API access token. Can also be set with MEVSPACE_API_TOKEN.",
				Optional:    true,
				Sensitive:   true,
			},
			"endpoint": schema.StringAttribute{
				Description: "API endpoint. Defaults to " + client.DefaultEndpoint + " or MEVSPACE_ENDPOINT.",
				Optional:    true,
			},
		},
	}
}

func (p *mevspaceProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token := os.Getenv("MEVSPACE_API_TOKEN")
	if !cfg.APIToken.IsNull() && !cfg.APIToken.IsUnknown() {
		token = cfg.APIToken.ValueString()
	}
	endpoint := os.Getenv("MEVSPACE_ENDPOINT")
	if !cfg.Endpoint.IsNull() && !cfg.Endpoint.IsUnknown() {
		endpoint = cfg.Endpoint.ValueString()
	}

	// Templates endpoint works without a token, so a missing token is only fatal when used.
	c := client.New(endpoint, token)
	c.UserAgent = "terraform-provider-mevspace/" + p.version
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *mevspaceProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newSSHKeyResource,
		newDedicatedServerResource,
	}
}

func (p *mevspaceProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newTemplatesDataSource,
		newStartupScriptsDataSource,
		newSSHKeysDataSource,
		newDedicatedServersDataSource,
		newDedicatedServerDataSource,
	}
}
