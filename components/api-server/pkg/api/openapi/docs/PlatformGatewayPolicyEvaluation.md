# PlatformGatewayPolicyEvaluation

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Valid** | Pointer to **bool** |  | [optional]
**ForbidsDefaults** | Pointer to **bool** |  | [optional]
**Diagnostics** | Pointer to [**[]PolicyDiagnostic**](PolicyDiagnostic.md) |  | [optional]
**Rendered** | Pointer to [**[]PolicyRenderedRule**](PolicyRenderedRule.md) |  | [optional]
**Defaults** | Pointer to [**FleetDefaults**](FleetDefaults.md) |  | [optional]
**SampleAllowed** | Pointer to **bool** |  | [optional]
**RejectingStatements** | Pointer to **[]int32** |  | [optional]
**DenialText** | Pointer to **string** |  | [optional]

## Methods

### NewPlatformGatewayPolicyEvaluation

`func NewPlatformGatewayPolicyEvaluation() *PlatformGatewayPolicyEvaluation`

NewPlatformGatewayPolicyEvaluation instantiates a new PlatformGatewayPolicyEvaluation object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPlatformGatewayPolicyEvaluationWithDefaults

`func NewPlatformGatewayPolicyEvaluationWithDefaults() *PlatformGatewayPolicyEvaluation`

NewPlatformGatewayPolicyEvaluationWithDefaults instantiates a new PlatformGatewayPolicyEvaluation object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetValid

`func (o *PlatformGatewayPolicyEvaluation) GetValid() bool`

GetValid returns the Valid field if non-nil, zero value otherwise.

### GetValidOk

`func (o *PlatformGatewayPolicyEvaluation) GetValidOk() (*bool, bool)`

GetValidOk returns a tuple with the Valid field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetValid

`func (o *PlatformGatewayPolicyEvaluation) SetValid(v bool)`

SetValid sets Valid field to given value.

### HasValid

`func (o *PlatformGatewayPolicyEvaluation) HasValid() bool`

HasValid returns a boolean if a field has been set.

### GetForbidsDefaults

`func (o *PlatformGatewayPolicyEvaluation) GetForbidsDefaults() bool`

GetForbidsDefaults returns the ForbidsDefaults field if non-nil, zero value otherwise.

### GetForbidsDefaultsOk

`func (o *PlatformGatewayPolicyEvaluation) GetForbidsDefaultsOk() (*bool, bool)`

GetForbidsDefaultsOk returns a tuple with the ForbidsDefaults field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetForbidsDefaults

`func (o *PlatformGatewayPolicyEvaluation) SetForbidsDefaults(v bool)`

SetForbidsDefaults sets ForbidsDefaults field to given value.

### HasForbidsDefaults

`func (o *PlatformGatewayPolicyEvaluation) HasForbidsDefaults() bool`

HasForbidsDefaults returns a boolean if a field has been set.

### GetDiagnostics

`func (o *PlatformGatewayPolicyEvaluation) GetDiagnostics() []PolicyDiagnostic`

GetDiagnostics returns the Diagnostics field if non-nil, zero value otherwise.

### GetDiagnosticsOk

`func (o *PlatformGatewayPolicyEvaluation) GetDiagnosticsOk() (*[]PolicyDiagnostic, bool)`

GetDiagnosticsOk returns a tuple with the Diagnostics field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDiagnostics

`func (o *PlatformGatewayPolicyEvaluation) SetDiagnostics(v []PolicyDiagnostic)`

SetDiagnostics sets Diagnostics field to given value.

### HasDiagnostics

`func (o *PlatformGatewayPolicyEvaluation) HasDiagnostics() bool`

HasDiagnostics returns a boolean if a field has been set.

### GetRendered

`func (o *PlatformGatewayPolicyEvaluation) GetRendered() []PolicyRenderedRule`

GetRendered returns the Rendered field if non-nil, zero value otherwise.

### GetRenderedOk

`func (o *PlatformGatewayPolicyEvaluation) GetRenderedOk() (*[]PolicyRenderedRule, bool)`

GetRenderedOk returns a tuple with the Rendered field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRendered

`func (o *PlatformGatewayPolicyEvaluation) SetRendered(v []PolicyRenderedRule)`

SetRendered sets Rendered field to given value.

### HasRendered

`func (o *PlatformGatewayPolicyEvaluation) HasRendered() bool`

HasRendered returns a boolean if a field has been set.

### GetDefaults

`func (o *PlatformGatewayPolicyEvaluation) GetDefaults() FleetDefaults`

GetDefaults returns the Defaults field if non-nil, zero value otherwise.

### GetDefaultsOk

`func (o *PlatformGatewayPolicyEvaluation) GetDefaultsOk() (*FleetDefaults, bool)`

GetDefaultsOk returns a tuple with the Defaults field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDefaults

`func (o *PlatformGatewayPolicyEvaluation) SetDefaults(v FleetDefaults)`

SetDefaults sets Defaults field to given value.

### HasDefaults

`func (o *PlatformGatewayPolicyEvaluation) HasDefaults() bool`

HasDefaults returns a boolean if a field has been set.

### GetSampleAllowed

`func (o *PlatformGatewayPolicyEvaluation) GetSampleAllowed() bool`

GetSampleAllowed returns the SampleAllowed field if non-nil, zero value otherwise.

### GetSampleAllowedOk

`func (o *PlatformGatewayPolicyEvaluation) GetSampleAllowedOk() (*bool, bool)`

GetSampleAllowedOk returns a tuple with the SampleAllowed field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSampleAllowed

`func (o *PlatformGatewayPolicyEvaluation) SetSampleAllowed(v bool)`

SetSampleAllowed sets SampleAllowed field to given value.

### HasSampleAllowed

`func (o *PlatformGatewayPolicyEvaluation) HasSampleAllowed() bool`

HasSampleAllowed returns a boolean if a field has been set.

### GetRejectingStatements

`func (o *PlatformGatewayPolicyEvaluation) GetRejectingStatements() []int32`

GetRejectingStatements returns the RejectingStatements field if non-nil, zero value otherwise.

### GetRejectingStatementsOk

`func (o *PlatformGatewayPolicyEvaluation) GetRejectingStatementsOk() (*[]int32, bool)`

GetRejectingStatementsOk returns a tuple with the RejectingStatements field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRejectingStatements

`func (o *PlatformGatewayPolicyEvaluation) SetRejectingStatements(v []int32)`

SetRejectingStatements sets RejectingStatements field to given value.

### HasRejectingStatements

`func (o *PlatformGatewayPolicyEvaluation) HasRejectingStatements() bool`

HasRejectingStatements returns a boolean if a field has been set.

### GetDenialText

`func (o *PlatformGatewayPolicyEvaluation) GetDenialText() string`

GetDenialText returns the DenialText field if non-nil, zero value otherwise.

### GetDenialTextOk

`func (o *PlatformGatewayPolicyEvaluation) GetDenialTextOk() (*string, bool)`

GetDenialTextOk returns a tuple with the DenialText field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDenialText

`func (o *PlatformGatewayPolicyEvaluation) SetDenialText(v string)`

SetDenialText sets DenialText field to given value.

### HasDenialText

`func (o *PlatformGatewayPolicyEvaluation) HasDenialText() bool`

HasDenialText returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
