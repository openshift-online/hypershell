# PlatformPolicySettings

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Kind** | Pointer to **string** |  | [optional]
**Mode** | Pointer to **string** | The global minimum enforcement mode (PLAT-PGP-06). Defaults to audit. | [optional]
**UpdatedAt** | Pointer to **time.Time** |  | [optional]
**UpdatedBy** | Pointer to **string** |  | [optional]
**SchemaVersion** | Pointer to **string** |  | [optional] [readonly]
**Defaults** | Pointer to [**PlatformPolicySettingsDefaults**](PlatformPolicySettingsDefaults.md) |  | [optional]
**DefaultsAcceptedAt** | Pointer to **time.Time** | When the api-server last accepted a fleet defaults value (PLAT-PGP-08). | [optional] [readonly]
**DefaultsRefusal** | Pointer to **string** | Why the configured fleet defaults are refused and the accepted defaults are still served, because an enforced policy forbids the new default request (PLAT-PGP-08.02). Empty when the configured defaults are accepted. | [optional] [readonly]

## Methods

### NewPlatformPolicySettings

`func NewPlatformPolicySettings() *PlatformPolicySettings`

NewPlatformPolicySettings instantiates a new PlatformPolicySettings object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPlatformPolicySettingsWithDefaults

`func NewPlatformPolicySettingsWithDefaults() *PlatformPolicySettings`

NewPlatformPolicySettingsWithDefaults instantiates a new PlatformPolicySettings object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetKind

`func (o *PlatformPolicySettings) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *PlatformPolicySettings) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *PlatformPolicySettings) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *PlatformPolicySettings) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetMode

`func (o *PlatformPolicySettings) GetMode() string`

GetMode returns the Mode field if non-nil, zero value otherwise.

### GetModeOk

`func (o *PlatformPolicySettings) GetModeOk() (*string, bool)`

GetModeOk returns a tuple with the Mode field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetMode

`func (o *PlatformPolicySettings) SetMode(v string)`

SetMode sets Mode field to given value.

### HasMode

`func (o *PlatformPolicySettings) HasMode() bool`

HasMode returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *PlatformPolicySettings) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *PlatformPolicySettings) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *PlatformPolicySettings) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *PlatformPolicySettings) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetUpdatedBy

`func (o *PlatformPolicySettings) GetUpdatedBy() string`

GetUpdatedBy returns the UpdatedBy field if non-nil, zero value otherwise.

### GetUpdatedByOk

`func (o *PlatformPolicySettings) GetUpdatedByOk() (*string, bool)`

GetUpdatedByOk returns a tuple with the UpdatedBy field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedBy

`func (o *PlatformPolicySettings) SetUpdatedBy(v string)`

SetUpdatedBy sets UpdatedBy field to given value.

### HasUpdatedBy

`func (o *PlatformPolicySettings) HasUpdatedBy() bool`

HasUpdatedBy returns a boolean if a field has been set.

### GetSchemaVersion

`func (o *PlatformPolicySettings) GetSchemaVersion() string`

GetSchemaVersion returns the SchemaVersion field if non-nil, zero value otherwise.

### GetSchemaVersionOk

`func (o *PlatformPolicySettings) GetSchemaVersionOk() (*string, bool)`

GetSchemaVersionOk returns a tuple with the SchemaVersion field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSchemaVersion

`func (o *PlatformPolicySettings) SetSchemaVersion(v string)`

SetSchemaVersion sets SchemaVersion field to given value.

### HasSchemaVersion

`func (o *PlatformPolicySettings) HasSchemaVersion() bool`

HasSchemaVersion returns a boolean if a field has been set.

### GetDefaults

`func (o *PlatformPolicySettings) GetDefaults() PlatformPolicySettingsDefaults`

GetDefaults returns the Defaults field if non-nil, zero value otherwise.

### GetDefaultsOk

`func (o *PlatformPolicySettings) GetDefaultsOk() (*PlatformPolicySettingsDefaults, bool)`

GetDefaultsOk returns a tuple with the Defaults field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDefaults

`func (o *PlatformPolicySettings) SetDefaults(v PlatformPolicySettingsDefaults)`

SetDefaults sets Defaults field to given value.

### HasDefaults

`func (o *PlatformPolicySettings) HasDefaults() bool`

HasDefaults returns a boolean if a field has been set.

### GetDefaultsAcceptedAt

`func (o *PlatformPolicySettings) GetDefaultsAcceptedAt() time.Time`

GetDefaultsAcceptedAt returns the DefaultsAcceptedAt field if non-nil, zero value otherwise.

### GetDefaultsAcceptedAtOk

`func (o *PlatformPolicySettings) GetDefaultsAcceptedAtOk() (*time.Time, bool)`

GetDefaultsAcceptedAtOk returns a tuple with the DefaultsAcceptedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDefaultsAcceptedAt

`func (o *PlatformPolicySettings) SetDefaultsAcceptedAt(v time.Time)`

SetDefaultsAcceptedAt sets DefaultsAcceptedAt field to given value.

### HasDefaultsAcceptedAt

`func (o *PlatformPolicySettings) HasDefaultsAcceptedAt() bool`

HasDefaultsAcceptedAt returns a boolean if a field has been set.

### GetDefaultsRefusal

`func (o *PlatformPolicySettings) GetDefaultsRefusal() string`

GetDefaultsRefusal returns the DefaultsRefusal field if non-nil, zero value otherwise.

### GetDefaultsRefusalOk

`func (o *PlatformPolicySettings) GetDefaultsRefusalOk() (*string, bool)`

GetDefaultsRefusalOk returns a tuple with the DefaultsRefusal field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDefaultsRefusal

`func (o *PlatformPolicySettings) SetDefaultsRefusal(v string)`

SetDefaultsRefusal sets DefaultsRefusal field to given value.

### HasDefaultsRefusal

`func (o *PlatformPolicySettings) HasDefaultsRefusal() bool`

HasDefaultsRefusal returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
