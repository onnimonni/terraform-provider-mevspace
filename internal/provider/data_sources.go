package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/onnimonni/terraform-provider-mevspace/internal/client"
)

// listDataSource is a data source returning every item of an API collection.
// Each item exposes id, name/label (when present) and all raw fields in attributes.
type listDataSource struct {
	client      *client.Client
	typeName    string
	description string
	filter      bool // adds optional "type" filter (templates)
	list        func(ctx context.Context, c *client.Client, filter string) ([]client.Object, error)
}

type listItemModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Attributes types.Map    `tfsdk:"attributes"`
}

type listModel struct {
	Type  types.String    `tfsdk:"type"`
	Items []listItemModel `tfsdk:"items"`
}

type listModelNoFilter struct {
	Items []listItemModel `tfsdk:"items"`
}

func newTemplatesDataSource() datasource.DataSource {
	return &listDataSource{
		typeName:    "_templates",
		description: "Available OS templates. Does not require an API token.",
		filter:      true,
		list: func(ctx context.Context, c *client.Client, f string) ([]client.Object, error) {
			return c.ListTemplates(ctx, f)
		},
	}
}

func newStartupScriptsDataSource() datasource.DataSource {
	return &listDataSource{
		typeName:    "_startup_scripts",
		description: "Startup scripts in the account.",
		list: func(ctx context.Context, c *client.Client, _ string) ([]client.Object, error) {
			return c.ListStartupScripts(ctx)
		},
	}
}

func newSSHKeysDataSource() datasource.DataSource {
	return &listDataSource{
		typeName:    "_ssh_keys",
		description: "SSH keys in the account.",
		list: func(ctx context.Context, c *client.Client, _ string) ([]client.Object, error) {
			return c.ListSSHKeys(ctx)
		},
	}
}

func newDedicatedServersDataSource() datasource.DataSource {
	return &listDataSource{
		typeName:    "_dedicated_servers",
		description: "Dedicated servers in the account. id is the hostname when present.",
		list: func(ctx context.Context, c *client.Client, _ string) ([]client.Object, error) {
			return c.ListDedicateds(ctx)
		},
	}
}

func (d *listDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + d.typeName
}

func (d *listDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"items": schema.ListNestedAttribute{
			Computed: true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id":         schema.StringAttribute{Computed: true},
					"name":       schema.StringAttribute{Computed: true, Description: "name, label or hostname field."},
					"attributes": schema.MapAttribute{Computed: true, ElementType: types.StringType, Description: "All API fields as strings."},
				},
			},
		},
	}
	if d.filter {
		attrs["type"] = schema.StringAttribute{
			Optional:    true,
			Description: "Filter: dedicated, vps or vps_cloud.",
			Validators:  []validator.String{stringvalidator.OneOf("dedicated", "vps", "vps_cloud")},
		}
	}
	resp.Schema = schema.Schema{Description: d.description, Attributes: attrs}
}

func (d *listDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *listDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var filter types.String
	if d.filter {
		var cfg listModel
		resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
		filter = cfg.Type
	}
	if resp.Diagnostics.HasError() {
		return
	}

	objs, err := d.list(ctx, d.client, filter.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Listing failed", err.Error())
		return
	}
	items := make([]listItemModel, 0, len(objs))
	for _, o := range objs {
		items = append(items, listItemModel{
			ID:         types.StringValue(itemID(o)),
			Name:       itemName(o),
			Attributes: attributesMap(ctx, o, &resp.Diagnostics),
		})
	}

	if d.filter {
		resp.Diagnostics.Append(resp.State.Set(ctx, listModel{Type: filter, Items: items})...)
	} else {
		resp.Diagnostics.Append(resp.State.Set(ctx, listModelNoFilter{Items: items})...)
	}
}

func itemID(o client.Object) string {
	if id := o.String("id"); id != "" {
		return id
	}
	return o.String("hostname")
}

func itemName(o client.Object) types.String {
	for _, k := range []string{"name", "label", "hostname"} {
		if v := optionalString(o, k); !v.IsNull() {
			return v
		}
	}
	return types.StringNull()
}

// --- single dedicated server ---

type dedicatedServerDataSource struct {
	client *client.Client
}

type dedicatedServerDataModel struct {
	Hostname   types.String `tfsdk:"hostname"`
	Attributes types.Map    `tfsdk:"attributes"`
}

func newDedicatedServerDataSource() datasource.DataSource { return &dedicatedServerDataSource{} }

func (d *dedicatedServerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dedicated_server"
}

func (d *dedicatedServerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Details of a dedicated server.",
		Attributes: map[string]schema.Attribute{
			"hostname":   schema.StringAttribute{Required: true},
			"attributes": schema.MapAttribute{Computed: true, ElementType: types.StringType, Description: "All API fields as strings."},
		},
	}
}

func (d *dedicatedServerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *dedicatedServerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m dedicatedServerDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	obj, err := d.client.GetDedicated(ctx, m.Hostname.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading dedicated server failed", err.Error())
		return
	}
	m.Attributes = attributesMap(ctx, obj, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}
