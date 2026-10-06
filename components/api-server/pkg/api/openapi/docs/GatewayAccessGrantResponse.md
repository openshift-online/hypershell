# GatewayAccessGrantResponse

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**RoleBindingId** | **string** |  | 
**UserId** | **string** |  | 
**Username** | **string** |  | 
**Name** | Pointer to **NullableString** |  | [optional] 
**Email** | Pointer to **NullableString** |  | [optional] 
**Role** | [**GatewayAccessRole**](GatewayAccessRole.md) |  | 
**IsCreator** | **bool** |  | 
**GrantedAt** | **time.Time** |  | 

## Methods

### NewGatewayAccessGrantResponse

`func NewGatewayAccessGrantResponse(roleBindingId string, userId string, username string, role GatewayAccessRole, isCreator bool, grantedAt time.Time, ) *GatewayAccessGrantResponse`

NewGatewayAccessGrantResponse instantiates a new GatewayAccessGrantResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayAccessGrantResponseWithDefaults

`func NewGatewayAccessGrantResponseWithDefaults() *GatewayAccessGrantResponse`

NewGatewayAccessGrantResponseWithDefaults instantiates a new GatewayAccessGrantResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetRoleBindingId

`func (o *GatewayAccessGrantResponse) GetRoleBindingId() string`

GetRoleBindingId returns the RoleBindingId field if non-nil, zero value otherwise.

### GetRoleBindingIdOk

`func (o *GatewayAccessGrantResponse) GetRoleBindingIdOk() (*string, bool)`

GetRoleBindingIdOk returns a tuple with the RoleBindingId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRoleBindingId

`func (o *GatewayAccessGrantResponse) SetRoleBindingId(v string)`

SetRoleBindingId sets RoleBindingId field to given value.


### GetUserId

`func (o *GatewayAccessGrantResponse) GetUserId() string`

GetUserId returns the UserId field if non-nil, zero value otherwise.

### GetUserIdOk

`func (o *GatewayAccessGrantResponse) GetUserIdOk() (*string, bool)`

GetUserIdOk returns a tuple with the UserId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUserId

`func (o *GatewayAccessGrantResponse) SetUserId(v string)`

SetUserId sets UserId field to given value.


### GetUsername

`func (o *GatewayAccessGrantResponse) GetUsername() string`

GetUsername returns the Username field if non-nil, zero value otherwise.

### GetUsernameOk

`func (o *GatewayAccessGrantResponse) GetUsernameOk() (*string, bool)`

GetUsernameOk returns a tuple with the Username field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUsername

`func (o *GatewayAccessGrantResponse) SetUsername(v string)`

SetUsername sets Username field to given value.


### GetName

`func (o *GatewayAccessGrantResponse) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *GatewayAccessGrantResponse) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *GatewayAccessGrantResponse) SetName(v string)`

SetName sets Name field to given value.

### HasName

`func (o *GatewayAccessGrantResponse) HasName() bool`

HasName returns a boolean if a field has been set.

### SetNameNil

`func (o *GatewayAccessGrantResponse) SetNameNil(b bool)`

 SetNameNil sets the value for Name to be an explicit nil

### UnsetName
`func (o *GatewayAccessGrantResponse) UnsetName()`

UnsetName ensures that no value is present for Name, not even an explicit nil
### GetEmail

`func (o *GatewayAccessGrantResponse) GetEmail() string`

GetEmail returns the Email field if non-nil, zero value otherwise.

### GetEmailOk

`func (o *GatewayAccessGrantResponse) GetEmailOk() (*string, bool)`

GetEmailOk returns a tuple with the Email field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEmail

`func (o *GatewayAccessGrantResponse) SetEmail(v string)`

SetEmail sets Email field to given value.

### HasEmail

`func (o *GatewayAccessGrantResponse) HasEmail() bool`

HasEmail returns a boolean if a field has been set.

### SetEmailNil

`func (o *GatewayAccessGrantResponse) SetEmailNil(b bool)`

 SetEmailNil sets the value for Email to be an explicit nil

### UnsetEmail
`func (o *GatewayAccessGrantResponse) UnsetEmail()`

UnsetEmail ensures that no value is present for Email, not even an explicit nil
### GetRole

`func (o *GatewayAccessGrantResponse) GetRole() GatewayAccessRole`

GetRole returns the Role field if non-nil, zero value otherwise.

### GetRoleOk

`func (o *GatewayAccessGrantResponse) GetRoleOk() (*GatewayAccessRole, bool)`

GetRoleOk returns a tuple with the Role field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRole

`func (o *GatewayAccessGrantResponse) SetRole(v GatewayAccessRole)`

SetRole sets Role field to given value.


### GetIsCreator

`func (o *GatewayAccessGrantResponse) GetIsCreator() bool`

GetIsCreator returns the IsCreator field if non-nil, zero value otherwise.

### GetIsCreatorOk

`func (o *GatewayAccessGrantResponse) GetIsCreatorOk() (*bool, bool)`

GetIsCreatorOk returns a tuple with the IsCreator field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetIsCreator

`func (o *GatewayAccessGrantResponse) SetIsCreator(v bool)`

SetIsCreator sets IsCreator field to given value.


### GetGrantedAt

`func (o *GatewayAccessGrantResponse) GetGrantedAt() time.Time`

GetGrantedAt returns the GrantedAt field if non-nil, zero value otherwise.

### GetGrantedAtOk

`func (o *GatewayAccessGrantResponse) GetGrantedAtOk() (*time.Time, bool)`

GetGrantedAtOk returns a tuple with the GrantedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGrantedAt

`func (o *GatewayAccessGrantResponse) SetGrantedAt(v time.Time)`

SetGrantedAt sets GrantedAt field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


