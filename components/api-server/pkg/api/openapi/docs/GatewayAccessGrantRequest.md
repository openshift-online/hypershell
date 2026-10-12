# GatewayAccessGrantRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Username** | **string** |  | 
**Subject** | Pointer to **string** | Optional Keycloak subject reference returned by the directory search | [optional] 
**Role** | [**GatewayAccessRole**](GatewayAccessRole.md) |  | 

## Methods

### NewGatewayAccessGrantRequest

`func NewGatewayAccessGrantRequest(username string, role GatewayAccessRole, ) *GatewayAccessGrantRequest`

NewGatewayAccessGrantRequest instantiates a new GatewayAccessGrantRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayAccessGrantRequestWithDefaults

`func NewGatewayAccessGrantRequestWithDefaults() *GatewayAccessGrantRequest`

NewGatewayAccessGrantRequestWithDefaults instantiates a new GatewayAccessGrantRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetUsername

`func (o *GatewayAccessGrantRequest) GetUsername() string`

GetUsername returns the Username field if non-nil, zero value otherwise.

### GetUsernameOk

`func (o *GatewayAccessGrantRequest) GetUsernameOk() (*string, bool)`

GetUsernameOk returns a tuple with the Username field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUsername

`func (o *GatewayAccessGrantRequest) SetUsername(v string)`

SetUsername sets Username field to given value.


### GetSubject

`func (o *GatewayAccessGrantRequest) GetSubject() string`

GetSubject returns the Subject field if non-nil, zero value otherwise.

### GetSubjectOk

`func (o *GatewayAccessGrantRequest) GetSubjectOk() (*string, bool)`

GetSubjectOk returns a tuple with the Subject field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSubject

`func (o *GatewayAccessGrantRequest) SetSubject(v string)`

SetSubject sets Subject field to given value.

### HasSubject

`func (o *GatewayAccessGrantRequest) HasSubject() bool`

HasSubject returns a boolean if a field has been set.

### GetRole

`func (o *GatewayAccessGrantRequest) GetRole() GatewayAccessRole`

GetRole returns the Role field if non-nil, zero value otherwise.

### GetRoleOk

`func (o *GatewayAccessGrantRequest) GetRoleOk() (*GatewayAccessRole, bool)`

GetRoleOk returns a tuple with the Role field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRole

`func (o *GatewayAccessGrantRequest) SetRole(v GatewayAccessRole)`

SetRole sets Role field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


