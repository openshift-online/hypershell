# ManagedClusterHealthCondition

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** | Stable check name from the bounded check-name enumeration (PLAT-HLTH-02). |
**Status** | **string** | Status of this check on the last run. |
**LastTransitionAt** | Pointer to **time.Time** | When this condition&#39;s status last changed. | [optional]
**Message** | Pointer to **string** | Human-readable detail for this check. | [optional]

## Methods

### NewManagedClusterHealthCondition

`func NewManagedClusterHealthCondition(name string, status string, ) *ManagedClusterHealthCondition`

NewManagedClusterHealthCondition instantiates a new ManagedClusterHealthCondition object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewManagedClusterHealthConditionWithDefaults

`func NewManagedClusterHealthConditionWithDefaults() *ManagedClusterHealthCondition`

NewManagedClusterHealthConditionWithDefaults instantiates a new ManagedClusterHealthCondition object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *ManagedClusterHealthCondition) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *ManagedClusterHealthCondition) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *ManagedClusterHealthCondition) SetName(v string)`

SetName sets Name field to given value.


### GetStatus

`func (o *ManagedClusterHealthCondition) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *ManagedClusterHealthCondition) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *ManagedClusterHealthCondition) SetStatus(v string)`

SetStatus sets Status field to given value.


### GetLastTransitionAt

`func (o *ManagedClusterHealthCondition) GetLastTransitionAt() time.Time`

GetLastTransitionAt returns the LastTransitionAt field if non-nil, zero value otherwise.

### GetLastTransitionAtOk

`func (o *ManagedClusterHealthCondition) GetLastTransitionAtOk() (*time.Time, bool)`

GetLastTransitionAtOk returns a tuple with the LastTransitionAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLastTransitionAt

`func (o *ManagedClusterHealthCondition) SetLastTransitionAt(v time.Time)`

SetLastTransitionAt sets LastTransitionAt field to given value.

### HasLastTransitionAt

`func (o *ManagedClusterHealthCondition) HasLastTransitionAt() bool`

HasLastTransitionAt returns a boolean if a field has been set.

### GetMessage

`func (o *ManagedClusterHealthCondition) GetMessage() string`

GetMessage returns the Message field if non-nil, zero value otherwise.

### GetMessageOk

`func (o *ManagedClusterHealthCondition) GetMessageOk() (*string, bool)`

GetMessageOk returns a tuple with the Message field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMessage

`func (o *ManagedClusterHealthCondition) SetMessage(v string)`

SetMessage sets Message field to given value.

### HasMessage

`func (o *ManagedClusterHealthCondition) HasMessage() bool`

HasMessage returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
