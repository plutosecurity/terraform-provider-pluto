// Package provider implements the Pluto Terraform provider.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/plutosecurity/terraform-provider-pluto/internal/client"
)

type plutoProvider struct {
	version string
}

type providerModel struct {
	Host   types.String `tfsdk:"host"`
	APIKey types.String `tfsdk:"api_key"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &plutoProvider{version: version} }
}

func (p *plutoProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "pluto"
	resp.Version = p.version
}

func (p *plutoProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Pluto policies.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Optional:    true,
				Description: "Pluto host. Defaults to PLUTO_HOST.",
			},
			"api_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Pluto API key. Defaults to PLUTO_API_KEY.",
			},
		},
	}
}

func (p *plutoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	host := valueOrEnv(cfg.Host, "PLUTO_HOST")
	apiKey := valueOrEnv(cfg.APIKey, "PLUTO_API_KEY")
	if host == "" {
		resp.Diagnostics.AddAttributeError(path.Root("host"), "Missing Pluto host", "Set host or PLUTO_HOST.")
	}
	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "Missing Pluto API key", "Set api_key or PLUTO_API_KEY.")
	}
	if resp.Diagnostics.HasError() {
		return
	}
	c := client.New(host, apiKey)
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *plutoProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{NewPolicyResource}
}

func (p *plutoProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}

func valueOrEnv(v types.String, env string) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	return os.Getenv(env)
}
