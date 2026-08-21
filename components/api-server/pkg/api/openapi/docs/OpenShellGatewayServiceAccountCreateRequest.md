# OpenShellGatewayServiceAccountCreateRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** |  | 
**Description** | Pointer to **NullableString** |  | [optional] 
**CredentialType** | Pointer to **string** |  | [optional] [default to "client_secret"]
**Role** | Pointer to [**OpenShellGatewayServiceAccountRole**](OpenShellGatewayServiceAccountRole.md) |  | [optional] 
**ExpiresAt** | Pointer to **time.Time** |  | [optional] 

## Methods

### NewOpenShellGatewayServiceAccountCreateRequest

`func NewOpenShellGatewayServiceAccountCreateRequest(name string, ) *OpenShellGatewayServiceAccountCreateRequest`

NewOpenShellGatewayServiceAccountCreateRequest instantiates a new OpenShellGatewayServiceAccountCreateRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewOpenShellGatewayServiceAccountCreateRequestWithDefaults

`func NewOpenShellGatewayServiceAccountCreateRequestWithDefaults() *OpenShellGatewayServiceAccountCreateRequest`

NewOpenShellGatewayServiceAccountCreateRequestWithDefaults instantiates a new OpenShellGatewayServiceAccountCreateRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *OpenShellGatewayServiceAccountCreateRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *OpenShellGatewayServiceAccountCreateRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *OpenShellGatewayServiceAccountCreateRequest) SetName(v string)`

SetName sets Name field to given value.


### GetDescription

`func (o *OpenShellGatewayServiceAccountCreateRequest) GetDescription() string`

GetDescription returns the Description field if non-nil, zero value otherwise.

### GetDescriptionOk

`func (o *OpenShellGatewayServiceAccountCreateRequest) GetDescriptionOk() (*string, bool)`

GetDescriptionOk returns a tuple with the Description field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDescription

`func (o *OpenShellGatewayServiceAccountCreateRequest) SetDescription(v string)`

SetDescription sets Description field to given value.

### HasDescription

`func (o *OpenShellGatewayServiceAccountCreateRequest) HasDescription() bool`

HasDescription returns a boolean if a field has been set.

### SetDescriptionNil

`func (o *OpenShellGatewayServiceAccountCreateRequest) SetDescriptionNil(b bool)`

 SetDescriptionNil sets the value for Description to be an explicit nil

### UnsetDescription
`func (o *OpenShellGatewayServiceAccountCreateRequest) UnsetDescription()`

UnsetDescription ensures that no value is present for Description, not even an explicit nil
### GetCredentialType

`func (o *OpenShellGatewayServiceAccountCreateRequest) GetCredentialType() string`

GetCredentialType returns the CredentialType field if non-nil, zero value otherwise.

### GetCredentialTypeOk

`func (o *OpenShellGatewayServiceAccountCreateRequest) GetCredentialTypeOk() (*string, bool)`

GetCredentialTypeOk returns a tuple with the CredentialType field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCredentialType

`func (o *OpenShellGatewayServiceAccountCreateRequest) SetCredentialType(v string)`

SetCredentialType sets CredentialType field to given value.

### HasCredentialType

`func (o *OpenShellGatewayServiceAccountCreateRequest) HasCredentialType() bool`

HasCredentialType returns a boolean if a field has been set.

### GetRole

`func (o *OpenShellGatewayServiceAccountCreateRequest) GetRole() OpenShellGatewayServiceAccountRole`

GetRole returns the Role field if non-nil, zero value otherwise.

### GetRoleOk

`func (o *OpenShellGatewayServiceAccountCreateRequest) GetRoleOk() (*OpenShellGatewayServiceAccountRole, bool)`

GetRoleOk returns a tuple with the Role field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRole

`func (o *OpenShellGatewayServiceAccountCreateRequest) SetRole(v OpenShellGatewayServiceAccountRole)`

SetRole sets Role field to given value.

### HasRole

`func (o *OpenShellGatewayServiceAccountCreateRequest) HasRole() bool`

HasRole returns a boolean if a field has been set.

### GetExpiresAt

`func (o *OpenShellGatewayServiceAccountCreateRequest) GetExpiresAt() time.Time`

GetExpiresAt returns the ExpiresAt field if non-nil, zero value otherwise.

### GetExpiresAtOk

`func (o *OpenShellGatewayServiceAccountCreateRequest) GetExpiresAtOk() (*time.Time, bool)`

GetExpiresAtOk returns a tuple with the ExpiresAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExpiresAt

`func (o *OpenShellGatewayServiceAccountCreateRequest) SetExpiresAt(v time.Time)`

SetExpiresAt sets ExpiresAt field to given value.

### HasExpiresAt

`func (o *OpenShellGatewayServiceAccountCreateRequest) HasExpiresAt() bool`

HasExpiresAt returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


