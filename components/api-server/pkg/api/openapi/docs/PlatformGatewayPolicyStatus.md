# PlatformGatewayPolicyStatus

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Validation** | Pointer to [**PlatformGatewayPolicyValidation**](PlatformGatewayPolicyValidation.md) |  | [optional]
**Gateways** | Pointer to [**[]PlatformGatewayPolicyGatewayStatus**](PlatformGatewayPolicyGatewayStatus.md) |  | [optional]
**Rollout** | Pointer to [**PlatformGatewayPolicyStatusRollout**](PlatformGatewayPolicyStatusRollout.md) |  | [optional]
**FindingsLast24h** | Pointer to [**PolicyFindingCounts**](PolicyFindingCounts.md) |  | [optional]

## Methods

### NewPlatformGatewayPolicyStatus

`func NewPlatformGatewayPolicyStatus() *PlatformGatewayPolicyStatus`

NewPlatformGatewayPolicyStatus instantiates a new PlatformGatewayPolicyStatus object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPlatformGatewayPolicyStatusWithDefaults

`func NewPlatformGatewayPolicyStatusWithDefaults() *PlatformGatewayPolicyStatus`

NewPlatformGatewayPolicyStatusWithDefaults instantiates a new PlatformGatewayPolicyStatus object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetValidation

`func (o *PlatformGatewayPolicyStatus) GetValidation() PlatformGatewayPolicyValidation`

GetValidation returns the Validation field if non-nil, zero value otherwise.

### GetValidationOk

`func (o *PlatformGatewayPolicyStatus) GetValidationOk() (*PlatformGatewayPolicyValidation, bool)`

GetValidationOk returns a tuple with the Validation field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetValidation

`func (o *PlatformGatewayPolicyStatus) SetValidation(v PlatformGatewayPolicyValidation)`

SetValidation sets Validation field to given value.

### HasValidation

`func (o *PlatformGatewayPolicyStatus) HasValidation() bool`

HasValidation returns a boolean if a field has been set.

### GetGateways

`func (o *PlatformGatewayPolicyStatus) GetGateways() []PlatformGatewayPolicyGatewayStatus`

GetGateways returns the Gateways field if non-nil, zero value otherwise.

### GetGatewaysOk

`func (o *PlatformGatewayPolicyStatus) GetGatewaysOk() (*[]PlatformGatewayPolicyGatewayStatus, bool)`

GetGatewaysOk returns a tuple with the Gateways field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGateways

`func (o *PlatformGatewayPolicyStatus) SetGateways(v []PlatformGatewayPolicyGatewayStatus)`

SetGateways sets Gateways field to given value.

### HasGateways

`func (o *PlatformGatewayPolicyStatus) HasGateways() bool`

HasGateways returns a boolean if a field has been set.

### GetRollout

`func (o *PlatformGatewayPolicyStatus) GetRollout() PlatformGatewayPolicyStatusRollout`

GetRollout returns the Rollout field if non-nil, zero value otherwise.

### GetRolloutOk

`func (o *PlatformGatewayPolicyStatus) GetRolloutOk() (*PlatformGatewayPolicyStatusRollout, bool)`

GetRolloutOk returns a tuple with the Rollout field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRollout

`func (o *PlatformGatewayPolicyStatus) SetRollout(v PlatformGatewayPolicyStatusRollout)`

SetRollout sets Rollout field to given value.

### HasRollout

`func (o *PlatformGatewayPolicyStatus) HasRollout() bool`

HasRollout returns a boolean if a field has been set.

### GetFindingsLast24h

`func (o *PlatformGatewayPolicyStatus) GetFindingsLast24h() PolicyFindingCounts`

GetFindingsLast24h returns the FindingsLast24h field if non-nil, zero value otherwise.

### GetFindingsLast24hOk

`func (o *PlatformGatewayPolicyStatus) GetFindingsLast24hOk() (*PolicyFindingCounts, bool)`

GetFindingsLast24hOk returns a tuple with the FindingsLast24h field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetFindingsLast24h

`func (o *PlatformGatewayPolicyStatus) SetFindingsLast24h(v PolicyFindingCounts)`

SetFindingsLast24h sets FindingsLast24h field to given value.

### HasFindingsLast24h

`func (o *PlatformGatewayPolicyStatus) HasFindingsLast24h() bool`

HasFindingsLast24h returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
