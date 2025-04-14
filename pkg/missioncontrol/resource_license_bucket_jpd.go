package missioncontrol

import (
	"context"
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jfrog/terraform-provider-shared/util"
	utilfw "github.com/jfrog/terraform-provider-shared/util/fw"
	"github.com/samber/lo"
)

const (
	attachLicenseEndpoint = "mc/api/v1/buckets"
	getBucketsEndpoint    = "mc/api/v1/buckets/{name}/report"
)

var _ resource.Resource = &licenseBucketJPDResource{}

type licenseBucketJPDResource struct {
	ProviderData util.ProviderMetadata
	TypeName     string
}

func NewLicenseBucketJPDResource() resource.Resource {
	return &licenseBucketJPDResource{
		TypeName: "missioncontrol_license_bucket_jpd",
	}
}

func (r *licenseBucketJPDResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = r.TypeName
}

func (r *licenseBucketJPDResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"license_bucket_name": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "Name of the license bucket",
			},
			"jpd_id": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "a JPD id to bind to the license bucket",
			},
		},
		MarkdownDescription: "Attaches a license from a bucket to the specified JPD.",
	}
}

type AttachLicensePostRequestAPIModel struct {
	JPDID        string `json:"jpd_id"`
	LicenseCount string `json:"license_count"`
}

type licenseBucketJPDResourceModel struct {
	ID           types.String `tfsdk:"id"`
	BucketName   types.String `tfsdk:"license_bucket_name"`
	JPDID        types.String `tfsdk:"jpd_id"`
	LicenseCount types.String `tfsdk:"license_count"`
}

type ReleaseLicensePostRequestAPIModel struct {
	Name string `json:"name"`
}

type LicenseBucketJPDAPIModel struct {
	Name         string `json:"name"`
	LicenseCount string `json:"license_count"`
}

type licenseBucketJPDGetAPIModel struct {
	Identifier string                     `json:"identifier"`
	Name       string                     `json:"name"`
	JPDs       []LicenseBucketJPDAPIModel `json:"jpds"`
}

func (r *licenseBucketJPDResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}
	r.ProviderData = req.ProviderData.(util.ProviderMetadata)
}

func (r *licenseBucketJPDResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	go util.SendUsageResourceCreate(ctx, r.ProviderData.Client.R(), r.ProviderData.ProductId, r.TypeName)

	var plan licenseBucketJPDResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	request := AttachLicensePostRequestAPIModel{
		JPDID:        plan.JPDID.ValueString(),
		LicenseCount: plan.LicenseCount.ValueString(),
	}

	var result licenseBucketPostResponseAPIModel
	var response *resty.Response
	var err error

	response, err = r.ProviderData.Client.R().
		SetResult(&response).
		SetPathParam("name", state.BucketName.ValueString()).
		Get(getBucketsEndpoint)
	if err != nil {
		utilfw.UnableToCreateResourceError(resp, err.Error())
		return
	}

	if response.IsError() {
		utilfw.UnableToCreateResourceError(resp, response.String())
		return
	}

	// Convert from the API data model to the Terraform data model
	// and refresh any attribute values.
	resp.Diagnostics.Append(plan.fromAPIModel(ctx, &result)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *licenseBucketJPDResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	go util.SendUsageResourceRead(ctx, r.ProviderData.Client.R(), r.ProviderData.ProductId, r.TypeName)

	var state licenseBucketJPDResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var licenseBucket licenseBucketJPDGetAPIModel

	response, err := r.ProviderData.Client.R().
		SetResult(&licenseBucket).
		SetPathParam("name", state.BucketName.ValueString()).
		Get(getBucketsEndpoint)

	if err != nil {
		utilfw.UnableToRefreshResourceError(resp, err.Error())
		return
	}

	if response.IsError() {
		utilfw.UnableToRefreshResourceError(resp, response.String())
		return
	}
	licenseBucketJPD, ok := lo.Find(
		licenseBucket.JPDs,
		func(licenseBucket LicenseBucketJPDAPIModel) bool {
			return licenseBucket.Name == state.JPDID.ValueString()
		},
	)
	if !ok {
		utilfw.UnableToRefreshResourceError(resp, fmt.Sprintf("JPD binding for jpd %s with bucket %s can't be found", state.JPDID.ValueString(), state.BucketName.ValueString()))
		return
	}

	state.BucketName = types.StringValue(licenseBucket.Name)
	state.JPDID = types.StringValue(licenseBucketJPD.Name)
	state.LicenseCount = types.StringValue(licenseBucketJPD.LicenseCount)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *licenseBucketJPDResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// noop
}

func (r *licenseBucketJPDResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	go util.SendUsageResourceDelete(ctx, r.ProviderData.Client.R(), r.ProviderData.ProductId, r.TypeName)

	var state licenseBucketJPDResourceModel

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	response, err := r.ProviderData.Client.R().
		SetPathParam("name", state.Name.ValueString()).
		Delete(licenseBucketEndpoint)
	if err != nil {
		utilfw.UnableToDeleteResourceError(resp, err.Error())
		return
	}

	if response.IsError() {
		utilfw.UnableToDeleteResourceError(resp, response.String())
		return
	}

	// If the logic reaches here, it implicitly succeeded and will remove
	// the resource from state if there are no other errors.
}
