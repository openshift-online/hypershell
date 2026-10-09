# GatewayAccessCapabilities

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**CallerRole** | Pointer to [**GatewayAccessRole**](GatewayAccessRole.md) |  | [optional] 
**CanManageAccess** | **bool** |  | 
**CanManageOwners** | **bool** |  | 

## Methods

### NewGatewayAccessCapabilities

`func NewGatewayAccessCapabilities(canManageAccess bool, canManageOwners bool, ) *GatewayAccessCapabilities`

NewGatewayAccessCapabilities instantiates a new GatewayAccessCapabilities object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayAccessCapabilitiesWithDefaults

`func NewGatewayAccessCapabilitiesWithDefaults() *GatewayAccessCapabilities`

NewGatewayAccessCapabilitiesWithDefaults instantiates a new GatewayAccessCapabilities object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetCallerRole

`func (o *GatewayAccessCapabilities) GetCallerRole() GatewayAccessRole`

GetCallerRole returns the CallerRole field if non-nil, zero value otherwise.

### GetCallerRoleOk

`func (o *GatewayAccessCapabilities) GetCallerRoleOk() (*GatewayAccessRole, bool)`

GetCallerRoleOk returns a tuple with the CallerRole field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCallerRole

`func (o *GatewayAccessCapabilities) SetCallerRole(v GatewayAccessRole)`

SetCallerRole sets CallerRole field to given value.

### HasCallerRole

`func (o *GatewayAccessCapabilities) HasCallerRole() bool`

HasCallerRole returns a boolean if a field has been set.

### GetCanManageAccess

`func (o *GatewayAccessCapabilities) GetCanManageAccess() bool`

GetCanManageAccess returns the CanManageAccess field if non-nil, zero value otherwise.

### GetCanManageAccessOk

`func (o *GatewayAccessCapabilities) GetCanManageAccessOk() (*bool, bool)`

GetCanManageAccessOk returns a tuple with the CanManageAccess field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCanManageAccess

`func (o *GatewayAccessCapabilities) SetCanManageAccess(v bool)`

SetCanManageAccess sets CanManageAccess field to given value.


### GetCanManageOwners

`func (o *GatewayAccessCapabilities) GetCanManageOwners() bool`

GetCanManageOwners returns the CanManageOwners field if non-nil, zero value otherwise.

### GetCanManageOwnersOk

`func (o *GatewayAccessCapabilities) GetCanManageOwnersOk() (*bool, bool)`

GetCanManageOwnersOk returns a tuple with the CanManageOwners field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCanManageOwners

`func (o *GatewayAccessCapabilities) SetCanManageOwners(v bool)`

SetCanManageOwners sets CanManageOwners field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


