package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/onnimonni/terraform-provider-mevspace/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &sshKeyResource{}
	_ resource.ResourceWithImportState = &sshKeyResource{}
)

type sshKeyResource struct {
	client *client.Client
}

type sshKeyModel struct {
	ID    types.String `tfsdk:"id"`
	Label types.String `tfsdk:"label"`
	Value types.String `tfsdk:"value"`
}

func newSSHKeyResource() resource.Resource { return &sshKeyResource{} }

func (r *sshKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_key"
}

func (r *sshKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "SSH public key stored in the MEVSpace account.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "SSH key id.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"label": schema.StringAttribute{
				Required:    true,
				Description: "Label of the key.",
			},
			"value": schema.StringAttribute{
				Required:    true,
				Description: "Public key in OpenSSH format. Changing the key recreates it; whitespace/comment changes do not.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIf(replaceIfKeyChanged,
					"Replaced when the key changes; whitespace/comment changes update in place.",
					"Replaced when the key changes; whitespace/comment changes update in place.")},
			},
		},
	}
}

func (r *sshKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *sshKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sshKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key, err := r.client.CreateSSHKey(ctx, plan.Label.ValueString(), plan.Value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Creating SSH key failed", err.Error())
		return
	}
	plan.ID = types.StringValue(key.String("id"))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *sshKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sshKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key, err := r.client.GetSSHKey(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading SSH key failed", err.Error())
		return
	}
	if v, ok := key["label"]; ok {
		state.Label = types.StringValue(client.Stringify(v))
	}
	// API may normalize whitespace/comment; only take remote value when the key itself differs.
	if v, ok := key["value"]; ok {
		remote := client.Stringify(v)
		if client.KeyMaterial(remote) != client.KeyMaterial(state.Value.ValueString()) {
			state.Value = types.StringValue(remote)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *sshKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sshKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.UpdateSSHKey(ctx, plan.ID.ValueString(), plan.Label.ValueString()); err != nil {
		resp.Diagnostics.AddError("Updating SSH key failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *sshKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sshKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSSHKey(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Deleting SSH key failed", err.Error())
	}
}

func (r *sshKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// replaceIfKeyChanged forces replacement only when the key itself changes; whitespace or
// comment differences (e.g. file("key.pub") with a trailing newline after import) update in place.
func replaceIfKeyChanged(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = client.KeyMaterial(req.PlanValue.ValueString()) != client.KeyMaterial(req.StateValue.ValueString())
}
