# RoleBinding

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional] 
**Kind** | Pointer to **string** |  | [optional] 
**Href** | Pointer to **string** |  | [optional] 
**CreatedAt** | Pointer to **time.Time** |  | [optional] 
**UpdatedAt** | Pointer to **time.Time** |  | [optional] 
**RoleId** | **string** |  | 
**Scope** | **string** |  | 
**UserId** | Pointer to **string** |  | [optional] 
**GatewayId** | Pointer to **string** |  | [optional] 

## Methods

### NewRoleBinding

`func NewRoleBinding(roleId string, scope string, ) *RoleBinding`

NewRoleBinding instantiates a new RoleBinding object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewRoleBindingWithDefaults

`func NewRoleBindingWithDefaults() *RoleBinding`

NewRoleBindingWithDefaults instantiates a new RoleBinding object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *RoleBinding) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *RoleBinding) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *RoleBinding) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *RoleBinding) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *RoleBinding) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *RoleBinding) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *RoleBinding) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *RoleBinding) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *RoleBinding) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *RoleBinding) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *RoleBinding) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *RoleBinding) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *RoleBinding) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *RoleBinding) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *RoleBinding) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *RoleBinding) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *RoleBinding) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *RoleBinding) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *RoleBinding) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *RoleBinding) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetRoleId

`func (o *RoleBinding) GetRoleId() string`

GetRoleId returns the RoleId field if non-nil, zero value otherwise.

### GetRoleIdOk

`func (o *RoleBinding) GetRoleIdOk() (*string, bool)`

GetRoleIdOk returns a tuple with the RoleId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRoleId

`func (o *RoleBinding) SetRoleId(v string)`

SetRoleId sets RoleId field to given value.


### GetScope

`func (o *RoleBinding) GetScope() string`

GetScope returns the Scope field if non-nil, zero value otherwise.

### GetScopeOk

`func (o *RoleBinding) GetScopeOk() (*string, bool)`

GetScopeOk returns a tuple with the Scope field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetScope

`func (o *RoleBinding) SetScope(v string)`

SetScope sets Scope field to given value.


### GetUserId

`func (o *RoleBinding) GetUserId() string`

GetUserId returns the UserId field if non-nil, zero value otherwise.

### GetUserIdOk

`func (o *RoleBinding) GetUserIdOk() (*string, bool)`

GetUserIdOk returns a tuple with the UserId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUserId

`func (o *RoleBinding) SetUserId(v string)`

SetUserId sets UserId field to given value.

### HasUserId

`func (o *RoleBinding) HasUserId() bool`

HasUserId returns a boolean if a field has been set.

### GetGatewayId

`func (o *RoleBinding) GetGatewayId() string`

GetGatewayId returns the GatewayId field if non-nil, zero value otherwise.

### GetGatewayIdOk

`func (o *RoleBinding) GetGatewayIdOk() (*string, bool)`

GetGatewayIdOk returns a tuple with the GatewayId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGatewayId

`func (o *RoleBinding) SetGatewayId(v string)`

SetGatewayId sets GatewayId field to given value.

### HasGatewayId

`func (o *RoleBinding) HasGatewayId() bool`

HasGatewayId returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


