# PlatformGatewayPolicyValidation

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Valid** | Pointer to **bool** |  | [optional]
**SchemaVersion** | Pointer to **string** |  | [optional]
**Diagnostics** | Pointer to [**[]PolicyDiagnostic**](PolicyDiagnostic.md) |  | [optional]

## Methods

### NewPlatformGatewayPolicyValidation

`func NewPlatformGatewayPolicyValidation() *PlatformGatewayPolicyValidation`

NewPlatformGatewayPolicyValidation instantiates a new PlatformGatewayPolicyValidation object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewPlatformGatewayPolicyValidationWithDefaults

`func NewPlatformGatewayPolicyValidationWithDefaults() *PlatformGatewayPolicyValidation`

NewPlatformGatewayPolicyValidationWithDefaults instantiates a new PlatformGatewayPolicyValidation object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetValid

`func (o *PlatformGatewayPolicyValidation) GetValid() bool`

GetValid returns the Valid field if non-nil, zero value otherwise.

### GetValidOk

`func (o *PlatformGatewayPolicyValidation) GetValidOk() (*bool, bool)`

GetValidOk returns a tuple with the Valid field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetValid

`func (o *PlatformGatewayPolicyValidation) SetValid(v bool)`

SetValid sets Valid field to given value.

### HasValid

`func (o *PlatformGatewayPolicyValidation) HasValid() bool`

HasValid returns a boolean if a field has been set.

### GetSchemaVersion

`func (o *PlatformGatewayPolicyValidation) GetSchemaVersion() string`

GetSchemaVersion returns the SchemaVersion field if non-nil, zero value otherwise.

### GetSchemaVersionOk

`func (o *PlatformGatewayPolicyValidation) GetSchemaVersionOk() (*string, bool)`

GetSchemaVersionOk returns a tuple with the SchemaVersion field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSchemaVersion

`func (o *PlatformGatewayPolicyValidation) SetSchemaVersion(v string)`

SetSchemaVersion sets SchemaVersion field to given value.

### HasSchemaVersion

`func (o *PlatformGatewayPolicyValidation) HasSchemaVersion() bool`

HasSchemaVersion returns a boolean if a field has been set.

### GetDiagnostics

`func (o *PlatformGatewayPolicyValidation) GetDiagnostics() []PolicyDiagnostic`

GetDiagnostics returns the Diagnostics field if non-nil, zero value otherwise.

### GetDiagnosticsOk

`func (o *PlatformGatewayPolicyValidation) GetDiagnosticsOk() (*[]PolicyDiagnostic, bool)`

GetDiagnosticsOk returns a tuple with the Diagnostics field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDiagnostics

`func (o *PlatformGatewayPolicyValidation) SetDiagnostics(v []PolicyDiagnostic)`

SetDiagnostics sets Diagnostics field to given value.

### HasDiagnostics

`func (o *PlatformGatewayPolicyValidation) HasDiagnostics() bool`

HasDiagnostics returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
