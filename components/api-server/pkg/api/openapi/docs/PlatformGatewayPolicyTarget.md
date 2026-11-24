# PlatformGatewayPolicyTarget

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**All** | Pointer to **bool** | Apply to every gateway (minus exclude). Defaults to true when no gateway_ids are set. | [optional]
**GatewayIds** | Pointer to **[]string** | Apply only to these gateways (ignored when all is true) | [optional]
**Exclude** | Pointer to **[]string** | Gateways the policy never applies to | [optional]

## Methods

### NewPlatformGatewayPolicyTarget

`func NewPlatformGatewayPolicyTarget() *PlatformGatewayPolicyTarget`

NewPlatformGatewayPolicyTarget instantiates a new PlatformGatewayPolicyTarget object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPlatformGatewayPolicyTargetWithDefaults

`func NewPlatformGatewayPolicyTargetWithDefaults() *PlatformGatewayPolicyTarget`

NewPlatformGatewayPolicyTargetWithDefaults instantiates a new PlatformGatewayPolicyTarget object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetAll

`func (o *PlatformGatewayPolicyTarget) GetAll() bool`

GetAll returns the All field if non-nil, zero value otherwise.

### GetAllOk

`func (o *PlatformGatewayPolicyTarget) GetAllOk() (*bool, bool)`

GetAllOk returns a tuple with the All field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAll

`func (o *PlatformGatewayPolicyTarget) SetAll(v bool)`

SetAll sets All field to given value.

### HasAll

`func (o *PlatformGatewayPolicyTarget) HasAll() bool`

HasAll returns a boolean if a field has been set.

### GetGatewayIds

`func (o *PlatformGatewayPolicyTarget) GetGatewayIds() []string`

GetGatewayIds returns the GatewayIds field if non-nil, zero value otherwise.

### GetGatewayIdsOk

`func (o *PlatformGatewayPolicyTarget) GetGatewayIdsOk() (*[]string, bool)`

GetGatewayIdsOk returns a tuple with the GatewayIds field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGatewayIds

`func (o *PlatformGatewayPolicyTarget) SetGatewayIds(v []string)`

SetGatewayIds sets GatewayIds field to given value.

### HasGatewayIds

`func (o *PlatformGatewayPolicyTarget) HasGatewayIds() bool`

HasGatewayIds returns a boolean if a field has been set.

### GetExclude

`func (o *PlatformGatewayPolicyTarget) GetExclude() []string`

GetExclude returns the Exclude field if non-nil, zero value otherwise.

### GetExcludeOk

`func (o *PlatformGatewayPolicyTarget) GetExcludeOk() (*[]string, bool)`

GetExcludeOk returns a tuple with the Exclude field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExclude

`func (o *PlatformGatewayPolicyTarget) SetExclude(v []string)`

SetExclude sets Exclude field to given value.

### HasExclude

`func (o *PlatformGatewayPolicyTarget) HasExclude() bool`

HasExclude returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
