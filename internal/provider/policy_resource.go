package provider

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/plutosecurity/terraform-provider-pluto/internal/client"
)

var (
	_ resource.ResourceWithConfigure    = &policyResource{}
	_ resource.ResourceWithImportState  = &policyResource{}
	_ resource.ResourceWithUpgradeState = &policyResource{}
)

type policyResource struct {
	client *client.Client
}

type policyModel struct {
	ID         types.String `tfsdk:"id"`
	Definition types.String `tfsdk:"definition"`
	Version    types.String `tfsdk:"version"`
}

func NewPolicyResource() resource.Resource { return &policyResource{} }

func (r *policyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_policy"
}

func (r *policyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:     1,
		Description: "A Pluto policy, defined exactly as the dashboard's policy editor saves it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"definition": schema.StringAttribute{
				Required: true,
				Description: "The policy as JSON, in the shape the dashboard's policy editor reads and writes. " +
					"Pluto validates it with the editor's own rules. Import a policy built in the dashboard " +
					"to get a definition to start from.",
			},
			"version": schema.StringAttribute{
				Computed:    true,
				Description: "Pluto's version of the policy, sent back on update so a newer change isn't overwritten.",
			},
		},
	}
}

func (r *policyResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *policyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan policyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.client.CreateDefinition(ctx, json.RawMessage(plan.Definition.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Creating Pluto policy failed", err.Error())
		return
	}
	plan.ID, plan.Version = types.StringValue(saved.ID), types.StringValue(saved.Version)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *policyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state policyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.client.GetDefinition(ctx, state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading Pluto policy failed", err.Error())
		return
	}
	definition, d := r.differs(ctx, state.Definition, saved.Policy)
	resp.Diagnostics.Append(d...)
	if definition != nil {
		state.Definition = types.StringValue(string(definition))
	}
	state.Version = types.StringValue(saved.Version)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// differs returns Pluto's definition when it differs from the one in state, and nil when Pluto reads
// both the same. Pluto decides that: both go through its normalize, which writes out its defaults.
func (r *policyResource) differs(ctx context.Context, known types.String, read json.RawMessage) (json.RawMessage, diag.Diagnostics) {
	var diags diag.Diagnostics
	theirs, err := r.client.NormalizeDefinition(ctx, read)
	if err != nil {
		diags.AddError("Reading Pluto policy failed", err.Error())
		return nil, diags
	}
	if known.IsNull() || known.IsUnknown() {
		return theirs, diags
	}
	ours, err := r.client.NormalizeDefinition(ctx, json.RawMessage(known.ValueString()))
	if err != nil {
		diags.AddError("Reading Pluto policy failed", err.Error())
		return nil, diags
	}
	var a, b any
	if json.Unmarshal(ours, &a) == nil && json.Unmarshal(theirs, &b) == nil && reflect.DeepEqual(a, b) {
		return nil, diags
	}
	return theirs, diags
}

func (r *policyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state policyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	saved, err := r.client.UpdateDefinition(ctx, state.ID.ValueString(), state.Version.ValueString(), json.RawMessage(plan.Definition.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Updating Pluto policy failed", err.Error())
		return
	}
	plan.Version = types.StringValue(saved.Version)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *policyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state policyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteDefinition(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Deleting Pluto policy failed", err.Error())
	}
}

func (r *policyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// UpgradeState refuses state written by provider 0.1, whose pluto_policy is now pluto_policy_legacy.
func (r *policyResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{0: {StateUpgrader: func(_ context.Context, _ resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
		resp.Diagnostics.AddError("pluto_policy changed format in provider 0.2",
			"Rename this resource to pluto_policy_legacy and add a moved block from pluto_policy, "+
				"or replace it with the new pluto_policy and import the policy by its id.")
	}}}
}
