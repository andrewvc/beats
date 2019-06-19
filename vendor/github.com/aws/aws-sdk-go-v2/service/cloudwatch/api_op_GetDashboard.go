// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

package cloudwatch

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/internal/awsutil"
)

// Please also see https://docs.aws.amazon.com/goto/WebAPI/monitoring-2010-08-01/GetDashboardInput
type GetDashboardInput struct {
	_ struct{} `type:"structure"`

	// The name of the dashboard to be described.
	//
	// DashboardName is a required field
	DashboardName *string `type:"string" required:"true"`
}

// String returns the string representation
func (s GetDashboardInput) String() string {
	return awsutil.Prettify(s)
}

// Validate inspects the fields of the type to determine if they are valid.
func (s *GetDashboardInput) Validate() error {
	invalidParams := aws.ErrInvalidParams{Context: "GetDashboardInput"}

	if s.DashboardName == nil {
		invalidParams.Add(aws.NewErrParamRequired("DashboardName"))
	}

	if invalidParams.Len() > 0 {
		return invalidParams
	}
	return nil
}

// Please also see https://docs.aws.amazon.com/goto/WebAPI/monitoring-2010-08-01/GetDashboardOutput
type GetDashboardOutput struct {
	_ struct{} `type:"structure"`

	// The Amazon Resource Name (ARN) of the dashboard.
	DashboardArn *string `type:"string"`

	// The detailed information about the dashboard, including what widgets are
	// included and their location on the dashboard. For more information about
	// the DashboardBody syntax, see CloudWatch-Dashboard-Body-Structure.
	DashboardBody *string `type:"string"`

	// The name of the dashboard.
	DashboardName *string `type:"string"`
}

// String returns the string representation
func (s GetDashboardOutput) String() string {
	return awsutil.Prettify(s)
}

const opGetDashboard = "GetDashboard"

// GetDashboardRequest returns a request value for making API operation for
// Amazon CloudWatch.
//
// Displays the details of the dashboard that you specify.
//
// To copy an existing dashboard, use GetDashboard, and then use the data returned
// within DashboardBody as the template for the new dashboard when you call
// PutDashboard to create the copy.
//
//    // Example sending a request using GetDashboardRequest.
//    req := client.GetDashboardRequest(params)
//    resp, err := req.Send(context.TODO())
//    if err == nil {
//        fmt.Println(resp)
//    }
//
// Please also see https://docs.aws.amazon.com/goto/WebAPI/monitoring-2010-08-01/GetDashboard
func (c *Client) GetDashboardRequest(input *GetDashboardInput) GetDashboardRequest {
	op := &aws.Operation{
		Name:       opGetDashboard,
		HTTPMethod: "POST",
		HTTPPath:   "/",
	}

	if input == nil {
		input = &GetDashboardInput{}
	}

	req := c.newRequest(op, input, &GetDashboardOutput{})
	return GetDashboardRequest{Request: req, Input: input, Copy: c.GetDashboardRequest}
}

// GetDashboardRequest is the request type for the
// GetDashboard API operation.
type GetDashboardRequest struct {
	*aws.Request
	Input *GetDashboardInput
	Copy  func(*GetDashboardInput) GetDashboardRequest
}

// Send marshals and sends the GetDashboard API request.
func (r GetDashboardRequest) Send(ctx context.Context) (*GetDashboardResponse, error) {
	r.Request.SetContext(ctx)
	err := r.Request.Send()
	if err != nil {
		return nil, err
	}

	resp := &GetDashboardResponse{
		GetDashboardOutput: r.Request.Data.(*GetDashboardOutput),
		response:           &aws.Response{Request: r.Request},
	}

	return resp, nil
}

// GetDashboardResponse is the response type for the
// GetDashboard API operation.
type GetDashboardResponse struct {
	*GetDashboardOutput

	response *aws.Response
}

// SDKResponseMetdata returns the response metadata for the
// GetDashboard request.
func (r *GetDashboardResponse) SDKResponseMetdata() *aws.Response {
	return r.response
}
