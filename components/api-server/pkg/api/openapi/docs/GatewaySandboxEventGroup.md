# GatewaySandboxEventGroup

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**SandboxName** | **string** |  |
**ObservedAt** | **time.Time** |  |
**Events** | [**[]GatewaySandboxEvent**](GatewaySandboxEvent.md) |  |

## Methods

### NewGatewaySandboxEventGroup

`func NewGatewaySandboxEventGroup(sandboxName string, observedAt time.Time, events []GatewaySandboxEvent, ) *GatewaySandboxEventGroup`

NewGatewaySandboxEventGroup instantiates a new GatewaySandboxEventGroup object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewaySandboxEventGroupWithDefaults

`func NewGatewaySandboxEventGroupWithDefaults() *GatewaySandboxEventGroup`

NewGatewaySandboxEventGroupWithDefaults instantiates a new GatewaySandboxEventGroup object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetSandboxName

`func (o *GatewaySandboxEventGroup) GetSandboxName() string`

GetSandboxName returns the SandboxName field if non-nil, zero value otherwise.

### GetSandboxNameOk

`func (o *GatewaySandboxEventGroup) GetSandboxNameOk() (*string, bool)`

GetSandboxNameOk returns a tuple with the SandboxName field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSandboxName

`func (o *GatewaySandboxEventGroup) SetSandboxName(v string)`

SetSandboxName sets SandboxName field to given value.


### GetObservedAt

`func (o *GatewaySandboxEventGroup) GetObservedAt() time.Time`

GetObservedAt returns the ObservedAt field if non-nil, zero value otherwise.

### GetObservedAtOk

`func (o *GatewaySandboxEventGroup) GetObservedAtOk() (*time.Time, bool)`

GetObservedAtOk returns a tuple with the ObservedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetObservedAt

`func (o *GatewaySandboxEventGroup) SetObservedAt(v time.Time)`

SetObservedAt sets ObservedAt field to given value.


### GetEvents

`func (o *GatewaySandboxEventGroup) GetEvents() []GatewaySandboxEvent`

GetEvents returns the Events field if non-nil, zero value otherwise.

### GetEventsOk

`func (o *GatewaySandboxEventGroup) GetEventsOk() (*[]GatewaySandboxEvent, bool)`

GetEventsOk returns a tuple with the Events field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEvents

`func (o *GatewaySandboxEventGroup) SetEvents(v []GatewaySandboxEvent)`

SetEvents sets Events field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
