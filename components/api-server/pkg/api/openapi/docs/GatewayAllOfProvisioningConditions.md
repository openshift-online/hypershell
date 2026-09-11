# GatewayAllOfProvisioningConditions

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Type** | Pointer to **string** | Condition identifier (e.g., EnvironmentReady) | [optional] 
**ConditionStatus** | Pointer to **string** | Current status of this provisioning step | [optional] 
**Message** | Pointer to **string** | Human-readable detail (failure reason when Failed) | [optional] 

## Methods

### NewGatewayAllOfProvisioningConditions

`func NewGatewayAllOfProvisioningConditions() *GatewayAllOfProvisioningConditions`

NewGatewayAllOfProvisioningConditions instantiates a new GatewayAllOfProvisioningConditions object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayAllOfProvisioningConditionsWithDefaults

`func NewGatewayAllOfProvisioningConditionsWithDefaults() *GatewayAllOfProvisioningConditions`

NewGatewayAllOfProvisioningConditionsWithDefaults instantiates a new GatewayAllOfProvisioningConditions object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetType

`func (o *GatewayAllOfProvisioningConditions) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *GatewayAllOfProvisioningConditions) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *GatewayAllOfProvisioningConditions) SetType(v string)`

SetType sets Type field to given value.

### HasType

`func (o *GatewayAllOfProvisioningConditions) HasType() bool`

HasType returns a boolean if a field has been set.

### GetConditionStatus

`func (o *GatewayAllOfProvisioningConditions) GetConditionStatus() string`

GetConditionStatus returns the ConditionStatus field if non-nil, zero value otherwise.

### GetConditionStatusOk

`func (o *GatewayAllOfProvisioningConditions) GetConditionStatusOk() (*string, bool)`

GetConditionStatusOk returns a tuple with the ConditionStatus field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetConditionStatus

`func (o *GatewayAllOfProvisioningConditions) SetConditionStatus(v string)`

SetConditionStatus sets ConditionStatus field to given value.

### HasConditionStatus

`func (o *GatewayAllOfProvisioningConditions) HasConditionStatus() bool`

HasConditionStatus returns a boolean if a field has been set.

### GetMessage

`func (o *GatewayAllOfProvisioningConditions) GetMessage() string`

GetMessage returns the Message field if non-nil, zero value otherwise.

### GetMessageOk

`func (o *GatewayAllOfProvisioningConditions) GetMessageOk() (*string, bool)`

GetMessageOk returns a tuple with the Message field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMessage

`func (o *GatewayAllOfProvisioningConditions) SetMessage(v string)`

SetMessage sets Message field to given value.

### HasMessage

`func (o *GatewayAllOfProvisioningConditions) HasMessage() bool`

HasMessage returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


