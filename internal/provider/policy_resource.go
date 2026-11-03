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
	_ resource.ResourceWithConfigure   = &policyResource{}
	_ resource.ResourceWithImportState = &policyResource{}
)

var selectorType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"type":   types.StringType,
	"values": types.SetType{ElemType: types.StringType},
}}

type policyResource struct {
	client *client.Client
}

type policyModel struct {
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

type selectorModel struct {
	Type   types.String `tfsdk:"type"`
	Values []string     `tfsdk:"values"`
}

func NewPolicyResource() resource.Resource { return &policyResource{} }

func (r *policyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_policy"
}

func (r *policyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A Pluto policy.",
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

func (r *policyResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*client.Client); ok {
		r.client = c
	}
}

func (r *policyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan policyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	body := toAPI(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreatePolicy(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Creating Pluto policy failed", err.Error())
		return
	}
	resp.Diagnostics.Append(fromAPI(ctx, created, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *policyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state policyModel
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
	resp.Diagnostics.Append(fromAPI(ctx, p, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *policyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan policyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	body := toAPI(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdatePolicy(ctx, plan.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Updating Pluto policy failed", err.Error())
		return
	}
	resp.Diagnostics.Append(fromAPI(ctx, updated, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *policyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state policyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeletePolicy(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Deleting Pluto policy failed", err.Error())
	}
}

func (r *policyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// toAPI builds the write body. An unknown tools or targeted_scopes means "not configured", which
// leaves the stored value alone.
func toAPI(ctx context.Context, m policyModel, diags *diag.Diagnostics) client.Policy {
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
		var selectors []selectorModel
		diags.Append(m.TargetedScopes.ElementsAs(ctx, &selectors, false)...)
		p.TargetedScopes = make([]client.Selector, 0, len(selectors))
		for _, s := range selectors {
			p.TargetedScopes = append(p.TargetedScopes, client.Selector{Type: s.Type.ValueString(), Values: s.Values})
		}
	}
	return p
}

func fromAPI(ctx context.Context, p *client.Policy, m *policyModel) diag.Diagnostics {
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

	selectors := make([]selectorModel, 0, len(p.TargetedScopes))
	for _, s := range p.TargetedScopes {
		selectors = append(selectors, selectorModel{Type: types.StringValue(s.Type), Values: s.Values})
	}
	scopes, d := types.ListValueFrom(ctx, selectorType, selectors)
	diags.Append(d...)
	m.TargetedScopes = scopes
	return diags
}
