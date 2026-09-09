# GatewayDeletionStatus

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**GatewayId** | **string** |  |
**ExternalReference** | **string** |  |
**State** | **string** | active, requested, or completed |
**DeletionRequestedAt** | Pointer to **time.Time** |  | [optional]
**DeletionCompletedAt** | Pointer to **time.Time** |  | [optional]

## Methods

### NewGatewayDeletionStatus

`func NewGatewayDeletionStatus(gatewayId string, externalReference string, state string, ) *GatewayDeletionStatus`

NewGatewayDeletionStatus instantiates a new GatewayDeletionStatus object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayDeletionStatusWithDefaults

`func NewGatewayDeletionStatusWithDefaults() *GatewayDeletionStatus`

NewGatewayDeletionStatusWithDefaults instantiates a new GatewayDeletionStatus object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetGatewayId

`func (o *GatewayDeletionStatus) GetGatewayId() string`

GetGatewayId returns the GatewayId field if non-nil, zero value otherwise.

### GetGatewayIdOk

`func (o *GatewayDeletionStatus) GetGatewayIdOk() (*string, bool)`

GetGatewayIdOk returns a tuple with the GatewayId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGatewayId

`func (o *GatewayDeletionStatus) SetGatewayId(v string)`

SetGatewayId sets GatewayId field to given value.


### GetExternalReference

`func (o *GatewayDeletionStatus) GetExternalReference() string`

GetExternalReference returns the ExternalReference field if non-nil, zero value otherwise.

### GetExternalReferenceOk

`func (o *GatewayDeletionStatus) GetExternalReferenceOk() (*string, bool)`

GetExternalReferenceOk returns a tuple with the ExternalReference field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExternalReference

`func (o *GatewayDeletionStatus) SetExternalReference(v string)`

SetExternalReference sets ExternalReference field to given value.


### GetState

`func (o *GatewayDeletionStatus) GetState() string`

GetState returns the State field if non-nil, zero value otherwise.

### GetStateOk

`func (o *GatewayDeletionStatus) GetStateOk() (*string, bool)`

GetStateOk returns a tuple with the State field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetState

`func (o *GatewayDeletionStatus) SetState(v string)`

SetState sets State field to given value.


### GetDeletionRequestedAt

`func (o *GatewayDeletionStatus) GetDeletionRequestedAt() time.Time`

GetDeletionRequestedAt returns the DeletionRequestedAt field if non-nil, zero value otherwise.

### GetDeletionRequestedAtOk

`func (o *GatewayDeletionStatus) GetDeletionRequestedAtOk() (*time.Time, bool)`

GetDeletionRequestedAtOk returns a tuple with the DeletionRequestedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDeletionRequestedAt

`func (o *GatewayDeletionStatus) SetDeletionRequestedAt(v time.Time)`

SetDeletionRequestedAt sets DeletionRequestedAt field to given value.

### HasDeletionRequestedAt

`func (o *GatewayDeletionStatus) HasDeletionRequestedAt() bool`

HasDeletionRequestedAt returns a boolean if a field has been set.

### GetDeletionCompletedAt

`func (o *GatewayDeletionStatus) GetDeletionCompletedAt() time.Time`

GetDeletionCompletedAt returns the DeletionCompletedAt field if non-nil, zero value otherwise.

### GetDeletionCompletedAtOk

`func (o *GatewayDeletionStatus) GetDeletionCompletedAtOk() (*time.Time, bool)`

GetDeletionCompletedAtOk returns a tuple with the DeletionCompletedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDeletionCompletedAt

`func (o *GatewayDeletionStatus) SetDeletionCompletedAt(v time.Time)`

SetDeletionCompletedAt sets DeletionCompletedAt field to given value.

### HasDeletionCompletedAt

`func (o *GatewayDeletionStatus) HasDeletionCompletedAt() bool`

HasDeletionCompletedAt returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


