package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/onnimonni/terraform-provider-mevspace/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &dedicatedServerResource{}
	_ resource.ResourceWithImportState = &dedicatedServerResource{}
)

type dedicatedServerResource struct {
	client *client.Client
}

type dedicatedServerModel struct {
	ID                types.String `tfsdk:"id"`
	Hostname          types.String `tfsdk:"hostname"`
	TemplateID        types.Int64  `tfsdk:"template_id"`
	RaidValue         types.String `tfsdk:"raid_value"`
	StartupScriptID   types.Int64  `tfsdk:"startup_script_id"`
	ReinstallTriggers types.Map    `tfsdk:"reinstall_triggers"`
	PowerState        types.String `tfsdk:"power_state"`
	Attributes        types.Map    `tfsdk:"attributes"`
}

func newDedicatedServerResource() resource.Resource { return &dedicatedServerResource{} }

func (r *dedicatedServerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dedicated_server"
}

func (r *dedicatedServerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an existing (already ordered) MEVSpace dedicated server: OS install and power state. " +
			"The API cannot order or cancel servers, so destroying this resource only removes it from state. " +
			"WARNING: setting or changing template_id, raid_value, startup_script_id or reinstall_triggers REINSTALLS the server and wipes its disks.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Same as hostname.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"hostname": schema.StringAttribute{
				Required:      true,
				Description:   "Hostname of the dedicated server, as shown in the MEVSpace panel.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"template_id": schema.Int64Attribute{
				Optional:    true,
				Description: "OS template id (see mevspace_templates). When set, the server is reinstalled on create and whenever install settings change.",
			},
			"raid_value": schema.StringAttribute{
				Optional:    true,
				Description: "RAID type used on reinstall.",
			},
			"startup_script_id": schema.Int64Attribute{
				Optional:    true,
				Description: "Startup script id (see mevspace_startup_scripts) run after reinstall.",
			},
			"reinstall_triggers": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Arbitrary values; any change triggers a reinstall (requires template_id).",
			},
			"power_state": schema.StringAttribute{
				Optional:    true,
				Description: "Desired power state: running or stopped. Applied on create and when changed; not read back from the API.",
				Validators:  []validator.String{stringvalidator.OneOf("running", "stopped")},
			},
			"attributes": schema.MapAttribute{
				Computed:      true,
				ElementType:   types.StringType,
				Description:   "All fields returned by the API for this server, as strings.",
				PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *dedicatedServerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *dedicatedServerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dedicatedServerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostname := plan.Hostname.ValueString()

	if _, err := r.client.GetDedicated(ctx, hostname); err != nil {
		resp.Diagnostics.AddError("Dedicated server not found", "Servers must be ordered in the MEVSpace panel first: "+err.Error())
		return
	}
	if !plan.TemplateID.IsNull() {
		if !r.reinstall(ctx, plan, &resp.Diagnostics) {
			return
		}
	}
	if !plan.PowerState.IsNull() {
		if err := r.client.PowerDedicated(ctx, hostname, powerAction(plan.PowerState.ValueString())); err != nil {
			resp.Diagnostics.AddError("Setting power state failed", err.Error())
			return
		}
	}

	plan.ID = plan.Hostname
	r.refresh(ctx, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dedicatedServerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dedicatedServerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	obj, err := r.client.GetDedicated(ctx, state.Hostname.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading dedicated server failed", err.Error())
		return
	}
	state.ID = state.Hostname
	state.Attributes = attributesMap(ctx, obj, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *dedicatedServerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state dedicatedServerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	installChanged := !plan.TemplateID.Equal(state.TemplateID) ||
		!plan.RaidValue.Equal(state.RaidValue) ||
		!plan.StartupScriptID.Equal(state.StartupScriptID) ||
		!plan.ReinstallTriggers.Equal(state.ReinstallTriggers)
	if installChanged && !plan.TemplateID.IsNull() {
		if !r.reinstall(ctx, plan, &resp.Diagnostics) {
			return
		}
	}
	if !plan.PowerState.IsNull() && !plan.PowerState.Equal(state.PowerState) {
		if err := r.client.PowerDedicated(ctx, plan.Hostname.ValueString(), powerAction(plan.PowerState.ValueString())); err != nil {
			resp.Diagnostics.AddError("Setting power state failed", err.Error())
			return
		}
	}

	// attributes keeps its planned (prior state) value; next refresh updates it.
	plan.ID = plan.Hostname
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dedicatedServerResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning("Dedicated server not cancelled",
		"The MEVSpace API cannot cancel servers. It was only removed from state; cancel it in the MEVSpace panel if needed.")
}

func (r *dedicatedServerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("hostname"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// reinstall wipes the server with the plan's install settings.
func (r *dedicatedServerResource) reinstall(ctx context.Context, plan dedicatedServerModel, diags *diag.Diagnostics) bool {
	err := r.client.ReinstallDedicated(ctx, plan.Hostname.ValueString(), client.ReinstallOptions{
		TemplateID:      plan.TemplateID.ValueInt64(),
		RaidValue:       plan.RaidValue.ValueString(),
		StartupScriptID: plan.StartupScriptID.ValueInt64(),
	})
	if err != nil {
		diags.AddError("Reinstalling dedicated server failed", err.Error())
		return false
	}
	return true
}

// refresh stores API fields; failure is only a warning since the action itself succeeded.
func (r *dedicatedServerResource) refresh(ctx context.Context, m *dedicatedServerModel, diags *diag.Diagnostics) {
	obj, err := r.client.GetDedicated(ctx, m.Hostname.ValueString())
	if err != nil {
		diags.AddWarning("Refreshing dedicated server failed", err.Error())
		obj = client.Object{}
	}
	m.Attributes = attributesMap(ctx, obj, diags)
}

func powerAction(state string) string {
	if state == "stopped" {
		return "stop"
	}
	return "start"
}
