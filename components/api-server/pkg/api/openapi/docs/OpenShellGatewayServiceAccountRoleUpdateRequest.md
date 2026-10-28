# OpenShellGatewayServiceAccountRoleUpdateRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Role** | [**OpenShellGatewayServiceAccountRole**](OpenShellGatewayServiceAccountRole.md) |  |
**Description** | Pointer to **NullableString** |  | [optional]

## Methods

### NewOpenShellGatewayServiceAccountRoleUpdateRequest

`func NewOpenShellGatewayServiceAccountRoleUpdateRequest(role OpenShellGatewayServiceAccountRole, ) *OpenShellGatewayServiceAccountRoleUpdateRequest`

NewOpenShellGatewayServiceAccountRoleUpdateRequest instantiates a new OpenShellGatewayServiceAccountRoleUpdateRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewOpenShellGatewayServiceAccountRoleUpdateRequestWithDefaults

`func NewOpenShellGatewayServiceAccountRoleUpdateRequestWithDefaults() *OpenShellGatewayServiceAccountRoleUpdateRequest`

NewOpenShellGatewayServiceAccountRoleUpdateRequestWithDefaults instantiates a new OpenShellGatewayServiceAccountRoleUpdateRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetRole

`func (o *OpenShellGatewayServiceAccountRoleUpdateRequest) GetRole() OpenShellGatewayServiceAccountRole`

GetRole returns the Role field if non-nil, zero value otherwise.

### GetRoleOk

`func (o *OpenShellGatewayServiceAccountRoleUpdateRequest) GetRoleOk() (*OpenShellGatewayServiceAccountRole, bool)`

GetRoleOk returns a tuple with the Role field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRole

`func (o *OpenShellGatewayServiceAccountRoleUpdateRequest) SetRole(v OpenShellGatewayServiceAccountRole)`

SetRole sets Role field to given value.


### GetDescription

`func (o *OpenShellGatewayServiceAccountRoleUpdateRequest) GetDescription() string`

GetDescription returns the Description field if non-nil, zero value otherwise.

### GetDescriptionOk

`func (o *OpenShellGatewayServiceAccountRoleUpdateRequest) GetDescriptionOk() (*string, bool)`

GetDescriptionOk returns a tuple with the Description field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDescription

`func (o *OpenShellGatewayServiceAccountRoleUpdateRequest) SetDescription(v string)`

SetDescription sets Description field to given value.

### HasDescription

`func (o *OpenShellGatewayServiceAccountRoleUpdateRequest) HasDescription() bool`

HasDescription returns a boolean if a field has been set.

### SetDescriptionNil

`func (o *OpenShellGatewayServiceAccountRoleUpdateRequest) SetDescriptionNil(b bool)`

 SetDescriptionNil sets the value for Description to be an explicit nil

### UnsetDescription
`func (o *OpenShellGatewayServiceAccountRoleUpdateRequest) UnsetDescription()`

UnsetDescription ensures that no value is present for Description, not even an explicit nil

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
