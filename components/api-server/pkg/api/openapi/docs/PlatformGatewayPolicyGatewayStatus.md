# PlatformGatewayPolicyGatewayStatus

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**GatewayId** | Pointer to **string** |  | [optional]
**Condition** | Pointer to **string** |  | [optional]
**Revision** | Pointer to **string** | The bundle revision the control plane wrote for the gateway | [optional]
**AppliedRevision** | Pointer to **string** | The bundle revision the gateway&#39;s interceptor last reported as applied | [optional]
**EffectiveMode** | Pointer to **string** |  | [optional]
**Message** | Pointer to **string** |  | [optional]
**NonCompliantSandboxes** | Pointer to **[]string** | Running sandboxes on the gateway that an enforced policy would reject (PLAT-PGP-16) | [optional]
**ObservedAt** | Pointer to **string** | RFC 3339 time the control plane last observed this rollout entry | [optional]
**WouldReject24h** | Pointer to **int32** |  | [optional] [readonly]

## Methods

### NewPlatformGatewayPolicyGatewayStatus

`func NewPlatformGatewayPolicyGatewayStatus() *PlatformGatewayPolicyGatewayStatus`

NewPlatformGatewayPolicyGatewayStatus instantiates a new PlatformGatewayPolicyGatewayStatus object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPlatformGatewayPolicyGatewayStatusWithDefaults

`func NewPlatformGatewayPolicyGatewayStatusWithDefaults() *PlatformGatewayPolicyGatewayStatus`

NewPlatformGatewayPolicyGatewayStatusWithDefaults instantiates a new PlatformGatewayPolicyGatewayStatus object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetGatewayId

`func (o *PlatformGatewayPolicyGatewayStatus) GetGatewayId() string`

GetGatewayId returns the GatewayId field if non-nil, zero value otherwise.

### GetGatewayIdOk

`func (o *PlatformGatewayPolicyGatewayStatus) GetGatewayIdOk() (*string, bool)`

GetGatewayIdOk returns a tuple with the GatewayId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGatewayId

`func (o *PlatformGatewayPolicyGatewayStatus) SetGatewayId(v string)`

SetGatewayId sets GatewayId field to given value.

### HasGatewayId

`func (o *PlatformGatewayPolicyGatewayStatus) HasGatewayId() bool`

HasGatewayId returns a boolean if a field has been set.

### GetCondition

`func (o *PlatformGatewayPolicyGatewayStatus) GetCondition() string`

GetCondition returns the Condition field if non-nil, zero value otherwise.

### GetConditionOk

`func (o *PlatformGatewayPolicyGatewayStatus) GetConditionOk() (*string, bool)`

GetConditionOk returns a tuple with the Condition field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCondition

`func (o *PlatformGatewayPolicyGatewayStatus) SetCondition(v string)`

SetCondition sets Condition field to given value.

### HasCondition

`func (o *PlatformGatewayPolicyGatewayStatus) HasCondition() bool`

HasCondition returns a boolean if a field has been set.

### GetRevision

`func (o *PlatformGatewayPolicyGatewayStatus) GetRevision() string`

GetRevision returns the Revision field if non-nil, zero value otherwise.

### GetRevisionOk

`func (o *PlatformGatewayPolicyGatewayStatus) GetRevisionOk() (*string, bool)`

GetRevisionOk returns a tuple with the Revision field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRevision

`func (o *PlatformGatewayPolicyGatewayStatus) SetRevision(v string)`

SetRevision sets Revision field to given value.

### HasRevision

`func (o *PlatformGatewayPolicyGatewayStatus) HasRevision() bool`

HasRevision returns a boolean if a field has been set.

### GetAppliedRevision

`func (o *PlatformGatewayPolicyGatewayStatus) GetAppliedRevision() string`

GetAppliedRevision returns the AppliedRevision field if non-nil, zero value otherwise.

### GetAppliedRevisionOk

`func (o *PlatformGatewayPolicyGatewayStatus) GetAppliedRevisionOk() (*string, bool)`

GetAppliedRevisionOk returns a tuple with the AppliedRevision field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetAppliedRevision

`func (o *PlatformGatewayPolicyGatewayStatus) SetAppliedRevision(v string)`

SetAppliedRevision sets AppliedRevision field to given value.

### HasAppliedRevision

`func (o *PlatformGatewayPolicyGatewayStatus) HasAppliedRevision() bool`

HasAppliedRevision returns a boolean if a field has been set.

### GetEffectiveMode

`func (o *PlatformGatewayPolicyGatewayStatus) GetEffectiveMode() string`

GetEffectiveMode returns the EffectiveMode field if non-nil, zero value otherwise.

### GetEffectiveModeOk

`func (o *PlatformGatewayPolicyGatewayStatus) GetEffectiveModeOk() (*string, bool)`

GetEffectiveModeOk returns a tuple with the EffectiveMode field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetEffectiveMode

`func (o *PlatformGatewayPolicyGatewayStatus) SetEffectiveMode(v string)`

SetEffectiveMode sets EffectiveMode field to given value.

### HasEffectiveMode

`func (o *PlatformGatewayPolicyGatewayStatus) HasEffectiveMode() bool`

HasEffectiveMode returns a boolean if a field has been set.

### GetMessage

`func (o *PlatformGatewayPolicyGatewayStatus) GetMessage() string`

GetMessage returns the Message field if non-nil, zero value otherwise.

### GetMessageOk

`func (o *PlatformGatewayPolicyGatewayStatus) GetMessageOk() (*string, bool)`

GetMessageOk returns a tuple with the Message field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMessage

`func (o *PlatformGatewayPolicyGatewayStatus) SetMessage(v string)`

SetMessage sets Message field to given value.

### HasMessage

`func (o *PlatformGatewayPolicyGatewayStatus) HasMessage() bool`

HasMessage returns a boolean if a field has been set.

### GetNonCompliantSandboxes

`func (o *PlatformGatewayPolicyGatewayStatus) GetNonCompliantSandboxes() []string`

GetNonCompliantSandboxes returns the NonCompliantSandboxes field if non-nil, zero value otherwise.

### GetNonCompliantSandboxesOk

`func (o *PlatformGatewayPolicyGatewayStatus) GetNonCompliantSandboxesOk() (*[]string, bool)`

GetNonCompliantSandboxesOk returns a tuple with the NonCompliantSandboxes field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetNonCompliantSandboxes

`func (o *PlatformGatewayPolicyGatewayStatus) SetNonCompliantSandboxes(v []string)`

SetNonCompliantSandboxes sets NonCompliantSandboxes field to given value.

### HasNonCompliantSandboxes

`func (o *PlatformGatewayPolicyGatewayStatus) HasNonCompliantSandboxes() bool`

HasNonCompliantSandboxes returns a boolean if a field has been set.

### GetObservedAt

`func (o *PlatformGatewayPolicyGatewayStatus) GetObservedAt() string`

GetObservedAt returns the ObservedAt field if non-nil, zero value otherwise.

### GetObservedAtOk

`func (o *PlatformGatewayPolicyGatewayStatus) GetObservedAtOk() (*string, bool)`

GetObservedAtOk returns a tuple with the ObservedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetObservedAt

`func (o *PlatformGatewayPolicyGatewayStatus) SetObservedAt(v string)`

SetObservedAt sets ObservedAt field to given value.

### HasObservedAt

`func (o *PlatformGatewayPolicyGatewayStatus) HasObservedAt() bool`

HasObservedAt returns a boolean if a field has been set.

### GetWouldReject24h

`func (o *PlatformGatewayPolicyGatewayStatus) GetWouldReject24h() int32`

GetWouldReject24h returns the WouldReject24h field if non-nil, zero value otherwise.

### GetWouldReject24hOk

`func (o *PlatformGatewayPolicyGatewayStatus) GetWouldReject24hOk() (*int32, bool)`

GetWouldReject24hOk returns a tuple with the WouldReject24h field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetWouldReject24h

`func (o *PlatformGatewayPolicyGatewayStatus) SetWouldReject24h(v int32)`

SetWouldReject24h sets WouldReject24h field to given value.

### HasWouldReject24h

`func (o *PlatformGatewayPolicyGatewayStatus) HasWouldReject24h() bool`

HasWouldReject24h returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
