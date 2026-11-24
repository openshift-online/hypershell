# GatewayEffectivePolicies

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**GatewayId** | Pointer to **string** |  | [optional]
**GlobalMode** | Pointer to **string** |  | [optional]
**SchemaVersion** | Pointer to **string** |  | [optional]
**Defaults** | Pointer to [**FleetDefaults**](FleetDefaults.md) |  | [optional]
**Policies** | Pointer to [**[]EffectivePolicy**](EffectivePolicy.md) |  | [optional]
**Findings** | Pointer to [**[]PolicyAuditFinding**](PolicyAuditFinding.md) |  | [optional]
**FindingsLast24h** | Pointer to [**PolicyFindingCounts**](PolicyFindingCounts.md) |  | [optional]

## Methods

### NewGatewayEffectivePolicies

`func NewGatewayEffectivePolicies() *GatewayEffectivePolicies`

NewGatewayEffectivePolicies instantiates a new GatewayEffectivePolicies object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayEffectivePoliciesWithDefaults

`func NewGatewayEffectivePoliciesWithDefaults() *GatewayEffectivePolicies`

NewGatewayEffectivePoliciesWithDefaults instantiates a new GatewayEffectivePolicies object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetGatewayId

`func (o *GatewayEffectivePolicies) GetGatewayId() string`

GetGatewayId returns the GatewayId field if non-nil, zero value otherwise.

### GetGatewayIdOk

`func (o *GatewayEffectivePolicies) GetGatewayIdOk() (*string, bool)`

GetGatewayIdOk returns a tuple with the GatewayId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGatewayId

`func (o *GatewayEffectivePolicies) SetGatewayId(v string)`

SetGatewayId sets GatewayId field to given value.

### HasGatewayId

`func (o *GatewayEffectivePolicies) HasGatewayId() bool`

HasGatewayId returns a boolean if a field has been set.

### GetGlobalMode

`func (o *GatewayEffectivePolicies) GetGlobalMode() string`

GetGlobalMode returns the GlobalMode field if non-nil, zero value otherwise.

### GetGlobalModeOk

`func (o *GatewayEffectivePolicies) GetGlobalModeOk() (*string, bool)`

GetGlobalModeOk returns a tuple with the GlobalMode field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGlobalMode

`func (o *GatewayEffectivePolicies) SetGlobalMode(v string)`

SetGlobalMode sets GlobalMode field to given value.

### HasGlobalMode

`func (o *GatewayEffectivePolicies) HasGlobalMode() bool`

HasGlobalMode returns a boolean if a field has been set.

### GetSchemaVersion

`func (o *GatewayEffectivePolicies) GetSchemaVersion() string`

GetSchemaVersion returns the SchemaVersion field if non-nil, zero value otherwise.

### GetSchemaVersionOk

`func (o *GatewayEffectivePolicies) GetSchemaVersionOk() (*string, bool)`

GetSchemaVersionOk returns a tuple with the SchemaVersion field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSchemaVersion

`func (o *GatewayEffectivePolicies) SetSchemaVersion(v string)`

SetSchemaVersion sets SchemaVersion field to given value.

### HasSchemaVersion

`func (o *GatewayEffectivePolicies) HasSchemaVersion() bool`

HasSchemaVersion returns a boolean if a field has been set.

### GetDefaults

`func (o *GatewayEffectivePolicies) GetDefaults() FleetDefaults`

GetDefaults returns the Defaults field if non-nil, zero value otherwise.

### GetDefaultsOk

`func (o *GatewayEffectivePolicies) GetDefaultsOk() (*FleetDefaults, bool)`

GetDefaultsOk returns a tuple with the Defaults field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDefaults

`func (o *GatewayEffectivePolicies) SetDefaults(v FleetDefaults)`

SetDefaults sets Defaults field to given value.

### HasDefaults

`func (o *GatewayEffectivePolicies) HasDefaults() bool`

HasDefaults returns a boolean if a field has been set.

### GetPolicies

`func (o *GatewayEffectivePolicies) GetPolicies() []EffectivePolicy`

GetPolicies returns the Policies field if non-nil, zero value otherwise.

### GetPoliciesOk

`func (o *GatewayEffectivePolicies) GetPoliciesOk() (*[]EffectivePolicy, bool)`

GetPoliciesOk returns a tuple with the Policies field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPolicies

`func (o *GatewayEffectivePolicies) SetPolicies(v []EffectivePolicy)`

SetPolicies sets Policies field to given value.

### HasPolicies

`func (o *GatewayEffectivePolicies) HasPolicies() bool`

HasPolicies returns a boolean if a field has been set.

### GetFindings

`func (o *GatewayEffectivePolicies) GetFindings() []PolicyAuditFinding`

GetFindings returns the Findings field if non-nil, zero value otherwise.

### GetFindingsOk

`func (o *GatewayEffectivePolicies) GetFindingsOk() (*[]PolicyAuditFinding, bool)`

GetFindingsOk returns a tuple with the Findings field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetFindings

`func (o *GatewayEffectivePolicies) SetFindings(v []PolicyAuditFinding)`

SetFindings sets Findings field to given value.

### HasFindings

`func (o *GatewayEffectivePolicies) HasFindings() bool`

HasFindings returns a boolean if a field has been set.

### GetFindingsLast24h

`func (o *GatewayEffectivePolicies) GetFindingsLast24h() PolicyFindingCounts`

GetFindingsLast24h returns the FindingsLast24h field if non-nil, zero value otherwise.

### GetFindingsLast24hOk

`func (o *GatewayEffectivePolicies) GetFindingsLast24hOk() (*PolicyFindingCounts, bool)`

GetFindingsLast24hOk returns a tuple with the FindingsLast24h field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetFindingsLast24h

`func (o *GatewayEffectivePolicies) SetFindingsLast24h(v PolicyFindingCounts)`

SetFindingsLast24h sets FindingsLast24h field to given value.

### HasFindingsLast24h

`func (o *GatewayEffectivePolicies) HasFindingsLast24h() bool`

HasFindingsLast24h returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
