package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/plutosecurity/terraform-provider-pluto/internal/client"
)

var (
	_ resource.ResourceWithConfigure   = &legacyPolicyResource{}
	_ resource.ResourceWithImportState = &legacyPolicyResource{}
	_ resource.ResourceWithMoveState   = &legacyPolicyResource{}
)

var legacySelectorType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"type":   types.StringType,
	"values": types.SetType{ElemType: types.StringType},
}}

type legacyPolicyResource struct {
	client *client.Client
}

type legacyPolicyModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	Category        types.String `tfsdk:"category"`
	PolicyType      types.String `tfsdk:"policy_type"`
	EnforcementTool types.String `tfsdk:"enforcement_tool"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	CustomMessage   types.String `tfsdk:"custom_message"`
	NotifyEndUser   types.Bool   `tfsdk:"notify_end_user"`
	Tools           types.Set    `tfsdk:"tools"`
	TargetedScopes  types.List   `tfsdk:"targeted_scopes"`
	ManagedBy       types.String `tfsdk:"managed_by"`
}

type legacySelectorModel struct {
	Type   types.String `tfsdk:"type"`
	Values []string     `tfsdk:"values"`
}

func NewLegacyPolicyResource() resource.Resource { return &legacyPolicyResource{} }

func (r *legacyPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_policy_legacy"
}

func (r *legacyPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A Pluto policy in the 0.1 format. Use `pluto_policy` for new configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":        schema.StringAttribute{Required: true},
			"description": schema.StringAttribute{Optional: true},
			"category": schema.StringAttribute{
				Required:      true,
				Description:   "Changing it replaces the policy.",
				PlanModifiers: replace,
			},
			"policy_type": schema.StringAttribute{
				Required: true,
			},
			"enforcement_tool": schema.StringAttribute{
				Required:      true,
				Description:   "Changing it replaces the policy.",
				PlanModifiers: replace,
			},
			"enabled":         schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"custom_message":  schema.StringAttribute{Optional: true},
			"notify_end_user": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"tools": schema.SetAttribute{
				Optional:      true,
				Computed:      true,
				ElementType:   types.StringType,
				Description:   "Omit to leave the stored value unchanged.",
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"managed_by": schema.StringAttribute{
				Computed:    true,
				Default:     stringdefault.StaticString(client.ManagedBy),
				Description: "Always \"terraform\".",
			},
			"targeted_scopes": schema.ListNestedAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "Omit to leave the stored value unchanged.",
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"type":   schema.StringAttribute{Required: true},
					"values": schema.SetAttribute{Required: true, ElementType: types.StringType},
				}},
			},
		},
	}
}

func (r *legacyPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *legacyPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan legacyPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	body := legacyToAPI(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreatePolicy(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating Pluto policy failed", err.Error())
		return
	}
	resp.Diagnostics.Append(legacyFromAPI(ctx, created, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *legacyPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state legacyPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client.GetPolicy(ctx, state.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) || (err == nil && p.Status == client.StatusDeleting) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading Pluto policy failed", err.Error())
		return
	}
	resp.Diagnostics.Append(legacyFromAPI(ctx, p, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *legacyPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan legacyPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	body := legacyToAPI(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdatePolicy(ctx, plan.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Updating Pluto policy failed", err.Error())
		return
	}
	resp.Diagnostics.Append(legacyFromAPI(ctx, updated, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *legacyPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state legacyPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeletePolicy(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Deleting Pluto policy failed", err.Error())
	}
}

func (r *legacyPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// toAPI builds the write body. An unknown tools or targeted_scopes means "not configured", which
// leaves the stored value alone.
func legacyToAPI(ctx context.Context, m legacyPolicyModel, diags *diag.Diagnostics) client.Policy {
	p := client.Policy{
		Name:            m.Name.ValueString(),
		Description:     m.Description.ValueStringPointer(),
		Category:        m.Category.ValueString(),
		PolicyType:      m.PolicyType.ValueString(),
		EnforcementTool: m.EnforcementTool.ValueStringPointer(),
		Enabled:         m.Enabled.ValueBool(),
		CustomMessage:   m.CustomMessage.ValueStringPointer(),
		NotifyEndUser:   m.NotifyEndUser.ValueBool(),
	}
	if !m.Tools.IsUnknown() && !m.Tools.IsNull() {
		var names []string
		diags.Append(m.Tools.ElementsAs(ctx, &names, false)...)
		tools := make([]client.Tool, 0, len(names))
		for _, n := range names {
			tools = append(tools, client.Tool{Type: "tool", DisplayName: n})
		}
		p.SelectedTools = &tools
	}
	if !m.TargetedScopes.IsUnknown() && !m.TargetedScopes.IsNull() {
		var selectors []legacySelectorModel
		diags.Append(m.TargetedScopes.ElementsAs(ctx, &selectors, false)...)
		p.TargetedScopes = make([]client.Selector, 0, len(selectors))
		for _, s := range selectors {
			p.TargetedScopes = append(p.TargetedScopes, client.Selector{Type: s.Type.ValueString(), Values: s.Values})
		}
	}
	return p
}

func legacyFromAPI(ctx context.Context, p *client.Policy, m *legacyPolicyModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.Description = types.StringPointerValue(p.Description)
	m.Category = types.StringValue(p.Category)
	m.PolicyType = types.StringValue(p.PolicyType)
	m.EnforcementTool = types.StringPointerValue(p.EnforcementTool)
	m.Enabled = types.BoolValue(p.Enabled)
	m.CustomMessage = types.StringPointerValue(p.CustomMessage)
	m.NotifyEndUser = types.BoolValue(p.NotifyEndUser)
	m.ManagedBy = types.StringPointerValue(p.ManagedBy)

	names := []string{}
	if p.SelectedTools != nil {
		for _, t := range *p.SelectedTools {
			names = append(names, t.DisplayName)
		}
	}
	tools, d := types.SetValueFrom(ctx, types.StringType, names)
	diags.Append(d...)
	m.Tools = tools

	selectors := make([]legacySelectorModel, 0, len(p.TargetedScopes))
	for _, s := range p.TargetedScopes {
		selectors = append(selectors, legacySelectorModel{Type: types.StringValue(s.Type), Values: s.Values})
	}
	scopes, d := types.ListValueFrom(ctx, legacySelectorType, selectors)
	diags.Append(d...)
	m.TargetedScopes = scopes
	return diags
}

// MoveState takes over a pluto_policy written by provider 0.1, whose schema this resource keeps.
func (r *legacyPolicyResource) MoveState(ctx context.Context) []resource.StateMover {
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	return []resource.StateMover{{
		SourceSchema: &schemaResp.Schema,
		StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
			if req.SourceTypeName != "pluto_policy" || req.SourceState == nil {
				return
			}
			var moved legacyPolicyModel
			resp.Diagnostics.Append(req.SourceState.Get(ctx, &moved)...)
			resp.Diagnostics.Append(resp.TargetState.Set(ctx, moved)...)
		},
	}}
}
