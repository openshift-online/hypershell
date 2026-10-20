# GatewaySandboxEvent

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Type** | Pointer to **string** |  | [optional]
**Reason** | Pointer to **string** |  | [optional]
**Message** | Pointer to **string** |  | [optional]
**FirstObservedAt** | Pointer to **time.Time** |  | [optional]
**LastObservedAt** | Pointer to **time.Time** |  | [optional]
**Count** | **int32** |  |
**InvolvedPodName** | Pointer to **string** |  | [optional]
**InvolvedPodUid** | Pointer to **string** |  | [optional]

## Methods

### NewGatewaySandboxEvent

`func NewGatewaySandboxEvent(count int32, ) *GatewaySandboxEvent`

NewGatewaySandboxEvent instantiates a new GatewaySandboxEvent object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewaySandboxEventWithDefaults

`func NewGatewaySandboxEventWithDefaults() *GatewaySandboxEvent`

NewGatewaySandboxEventWithDefaults instantiates a new GatewaySandboxEvent object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetType

`func (o *GatewaySandboxEvent) GetType() string`

GetType returns the Type field if non-nil, zero value otherwise.

### GetTypeOk

`func (o *GatewaySandboxEvent) GetTypeOk() (*string, bool)`

GetTypeOk returns a tuple with the Type field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetType

`func (o *GatewaySandboxEvent) SetType(v string)`

SetType sets Type field to given value.

### HasType

`func (o *GatewaySandboxEvent) HasType() bool`

HasType returns a boolean if a field has been set.

### GetReason

`func (o *GatewaySandboxEvent) GetReason() string`

GetReason returns the Reason field if non-nil, zero value otherwise.

### GetReasonOk

`func (o *GatewaySandboxEvent) GetReasonOk() (*string, bool)`

GetReasonOk returns a tuple with the Reason field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetReason

`func (o *GatewaySandboxEvent) SetReason(v string)`

SetReason sets Reason field to given value.

### HasReason

`func (o *GatewaySandboxEvent) HasReason() bool`

HasReason returns a boolean if a field has been set.

### GetMessage

`func (o *GatewaySandboxEvent) GetMessage() string`

GetMessage returns the Message field if non-nil, zero value otherwise.

### GetMessageOk

`func (o *GatewaySandboxEvent) GetMessageOk() (*string, bool)`

GetMessageOk returns a tuple with the Message field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMessage

`func (o *GatewaySandboxEvent) SetMessage(v string)`

SetMessage sets Message field to given value.

### HasMessage

`func (o *GatewaySandboxEvent) HasMessage() bool`

HasMessage returns a boolean if a field has been set.

### GetFirstObservedAt

`func (o *GatewaySandboxEvent) GetFirstObservedAt() time.Time`

GetFirstObservedAt returns the FirstObservedAt field if non-nil, zero value otherwise.

### GetFirstObservedAtOk

`func (o *GatewaySandboxEvent) GetFirstObservedAtOk() (*time.Time, bool)`

GetFirstObservedAtOk returns a tuple with the FirstObservedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetFirstObservedAt

`func (o *GatewaySandboxEvent) SetFirstObservedAt(v time.Time)`

SetFirstObservedAt sets FirstObservedAt field to given value.

### HasFirstObservedAt

`func (o *GatewaySandboxEvent) HasFirstObservedAt() bool`

HasFirstObservedAt returns a boolean if a field has been set.

### GetLastObservedAt

`func (o *GatewaySandboxEvent) GetLastObservedAt() time.Time`

GetLastObservedAt returns the LastObservedAt field if non-nil, zero value otherwise.

### GetLastObservedAtOk

`func (o *GatewaySandboxEvent) GetLastObservedAtOk() (*time.Time, bool)`

GetLastObservedAtOk returns a tuple with the LastObservedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLastObservedAt

`func (o *GatewaySandboxEvent) SetLastObservedAt(v time.Time)`

SetLastObservedAt sets LastObservedAt field to given value.

### HasLastObservedAt

`func (o *GatewaySandboxEvent) HasLastObservedAt() bool`

HasLastObservedAt returns a boolean if a field has been set.

### GetCount

`func (o *GatewaySandboxEvent) GetCount() int32`

GetCount returns the Count field if non-nil, zero value otherwise.

### GetCountOk

`func (o *GatewaySandboxEvent) GetCountOk() (*int32, bool)`

GetCountOk returns a tuple with the Count field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCount

`func (o *GatewaySandboxEvent) SetCount(v int32)`

SetCount sets Count field to given value.


### GetInvolvedPodName

`func (o *GatewaySandboxEvent) GetInvolvedPodName() string`

GetInvolvedPodName returns the InvolvedPodName field if non-nil, zero value otherwise.

### GetInvolvedPodNameOk

`func (o *GatewaySandboxEvent) GetInvolvedPodNameOk() (*string, bool)`

GetInvolvedPodNameOk returns a tuple with the InvolvedPodName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetInvolvedPodName

`func (o *GatewaySandboxEvent) SetInvolvedPodName(v string)`

SetInvolvedPodName sets InvolvedPodName field to given value.

### HasInvolvedPodName

`func (o *GatewaySandboxEvent) HasInvolvedPodName() bool`

HasInvolvedPodName returns a boolean if a field has been set.

### GetInvolvedPodUid

`func (o *GatewaySandboxEvent) GetInvolvedPodUid() string`

GetInvolvedPodUid returns the InvolvedPodUid field if non-nil, zero value otherwise.

### GetInvolvedPodUidOk

`func (o *GatewaySandboxEvent) GetInvolvedPodUidOk() (*string, bool)`

GetInvolvedPodUidOk returns a tuple with the InvolvedPodUid field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetInvolvedPodUid

`func (o *GatewaySandboxEvent) SetInvolvedPodUid(v string)`

SetInvolvedPodUid sets InvolvedPodUid field to given value.

### HasInvolvedPodUid

`func (o *GatewaySandboxEvent) HasInvolvedPodUid() bool`

HasInvolvedPodUid returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
