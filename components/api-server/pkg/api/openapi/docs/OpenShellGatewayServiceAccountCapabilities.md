# OpenShellGatewayServiceAccountCapabilities

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**CanCreate** | **bool** |  | 
**AllowedRoles** | [**[]OpenShellGatewayServiceAccountRole**](OpenShellGatewayServiceAccountRole.md) |  | 
**CanManageAll** | **bool** |  | 
**ExpirationPolicy** | [**OpenShellGatewayServiceAccountExpirationPolicy**](OpenShellGatewayServiceAccountExpirationPolicy.md) |  | 

## Methods

### NewOpenShellGatewayServiceAccountCapabilities

`func NewOpenShellGatewayServiceAccountCapabilities(canCreate bool, allowedRoles []OpenShellGatewayServiceAccountRole, canManageAll bool, expirationPolicy OpenShellGatewayServiceAccountExpirationPolicy, ) *OpenShellGatewayServiceAccountCapabilities`

NewOpenShellGatewayServiceAccountCapabilities instantiates a new OpenShellGatewayServiceAccountCapabilities object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewOpenShellGatewayServiceAccountCapabilitiesWithDefaults

`func NewOpenShellGatewayServiceAccountCapabilitiesWithDefaults() *OpenShellGatewayServiceAccountCapabilities`

NewOpenShellGatewayServiceAccountCapabilitiesWithDefaults instantiates a new OpenShellGatewayServiceAccountCapabilities object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetCanCreate

`func (o *OpenShellGatewayServiceAccountCapabilities) GetCanCreate() bool`

GetCanCreate returns the CanCreate field if non-nil, zero value otherwise.

### GetCanCreateOk

`func (o *OpenShellGatewayServiceAccountCapabilities) GetCanCreateOk() (*bool, bool)`

GetCanCreateOk returns a tuple with the CanCreate field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCanCreate

`func (o *OpenShellGatewayServiceAccountCapabilities) SetCanCreate(v bool)`

SetCanCreate sets CanCreate field to given value.


### GetAllowedRoles

`func (o *OpenShellGatewayServiceAccountCapabilities) GetAllowedRoles() []OpenShellGatewayServiceAccountRole`

GetAllowedRoles returns the AllowedRoles field if non-nil, zero value otherwise.

### GetAllowedRolesOk

`func (o *OpenShellGatewayServiceAccountCapabilities) GetAllowedRolesOk() (*[]OpenShellGatewayServiceAccountRole, bool)`

GetAllowedRolesOk returns a tuple with the AllowedRoles field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAllowedRoles

`func (o *OpenShellGatewayServiceAccountCapabilities) SetAllowedRoles(v []OpenShellGatewayServiceAccountRole)`

SetAllowedRoles sets AllowedRoles field to given value.


### GetCanManageAll

`func (o *OpenShellGatewayServiceAccountCapabilities) GetCanManageAll() bool`

GetCanManageAll returns the CanManageAll field if non-nil, zero value otherwise.

### GetCanManageAllOk

`func (o *OpenShellGatewayServiceAccountCapabilities) GetCanManageAllOk() (*bool, bool)`

GetCanManageAllOk returns a tuple with the CanManageAll field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCanManageAll

`func (o *OpenShellGatewayServiceAccountCapabilities) SetCanManageAll(v bool)`

SetCanManageAll sets CanManageAll field to given value.


### GetExpirationPolicy

`func (o *OpenShellGatewayServiceAccountCapabilities) GetExpirationPolicy() OpenShellGatewayServiceAccountExpirationPolicy`

GetExpirationPolicy returns the ExpirationPolicy field if non-nil, zero value otherwise.

### GetExpirationPolicyOk

`func (o *OpenShellGatewayServiceAccountCapabilities) GetExpirationPolicyOk() (*OpenShellGatewayServiceAccountExpirationPolicy, bool)`

GetExpirationPolicyOk returns a tuple with the ExpirationPolicy field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExpirationPolicy

`func (o *OpenShellGatewayServiceAccountCapabilities) SetExpirationPolicy(v OpenShellGatewayServiceAccountExpirationPolicy)`

SetExpirationPolicy sets ExpirationPolicy field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


