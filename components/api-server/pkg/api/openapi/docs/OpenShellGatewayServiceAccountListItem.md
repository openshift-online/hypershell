# OpenShellGatewayServiceAccountListItem

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | **string** |  | 
**GatewayId** | **string** |  | 
**Name** | **string** |  | 
**Description** | Pointer to **NullableString** |  | [optional] 
**CredentialType** | **string** |  | 
**Role** | [**OpenShellGatewayServiceAccountRole**](OpenShellGatewayServiceAccountRole.md) |  | 
**Status** | [**OpenShellGatewayServiceAccountStatus**](OpenShellGatewayServiceAccountStatus.md) |  | 
**CreatedByUserId** | **string** |  | 
**ClientId** | **string** |  | 
**Subject** | **string** |  | 
**ExpiresAt** | **time.Time** |  | 
**RevokedAt** | Pointer to **NullableTime** |  | [optional] 
**LastError** | Pointer to **NullableString** |  | [optional] 
**CreatedAt** | **time.Time** |  | 
**UpdatedAt** | **time.Time** |  | 

## Methods

### NewOpenShellGatewayServiceAccountListItem

`func NewOpenShellGatewayServiceAccountListItem(id string, gatewayId string, name string, credentialType string, role OpenShellGatewayServiceAccountRole, status OpenShellGatewayServiceAccountStatus, createdByUserId string, clientId string, subject string, expiresAt time.Time, createdAt time.Time, updatedAt time.Time, ) *OpenShellGatewayServiceAccountListItem`

NewOpenShellGatewayServiceAccountListItem instantiates a new OpenShellGatewayServiceAccountListItem object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewOpenShellGatewayServiceAccountListItemWithDefaults

`func NewOpenShellGatewayServiceAccountListItemWithDefaults() *OpenShellGatewayServiceAccountListItem`

NewOpenShellGatewayServiceAccountListItemWithDefaults instantiates a new OpenShellGatewayServiceAccountListItem object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *OpenShellGatewayServiceAccountListItem) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *OpenShellGatewayServiceAccountListItem) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *OpenShellGatewayServiceAccountListItem) SetId(v string)`

SetId sets Id field to given value.


### GetGatewayId

`func (o *OpenShellGatewayServiceAccountListItem) GetGatewayId() string`

GetGatewayId returns the GatewayId field if non-nil, zero value otherwise.

### GetGatewayIdOk

`func (o *OpenShellGatewayServiceAccountListItem) GetGatewayIdOk() (*string, bool)`

GetGatewayIdOk returns a tuple with the GatewayId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGatewayId

`func (o *OpenShellGatewayServiceAccountListItem) SetGatewayId(v string)`

SetGatewayId sets GatewayId field to given value.


### GetName

`func (o *OpenShellGatewayServiceAccountListItem) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *OpenShellGatewayServiceAccountListItem) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *OpenShellGatewayServiceAccountListItem) SetName(v string)`

SetName sets Name field to given value.


### GetDescription

`func (o *OpenShellGatewayServiceAccountListItem) GetDescription() string`

GetDescription returns the Description field if non-nil, zero value otherwise.

### GetDescriptionOk

`func (o *OpenShellGatewayServiceAccountListItem) GetDescriptionOk() (*string, bool)`

GetDescriptionOk returns a tuple with the Description field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDescription

`func (o *OpenShellGatewayServiceAccountListItem) SetDescription(v string)`

SetDescription sets Description field to given value.

### HasDescription

`func (o *OpenShellGatewayServiceAccountListItem) HasDescription() bool`

HasDescription returns a boolean if a field has been set.

### SetDescriptionNil

`func (o *OpenShellGatewayServiceAccountListItem) SetDescriptionNil(b bool)`

 SetDescriptionNil sets the value for Description to be an explicit nil

### UnsetDescription
`func (o *OpenShellGatewayServiceAccountListItem) UnsetDescription()`

UnsetDescription ensures that no value is present for Description, not even an explicit nil
### GetCredentialType

`func (o *OpenShellGatewayServiceAccountListItem) GetCredentialType() string`

GetCredentialType returns the CredentialType field if non-nil, zero value otherwise.

### GetCredentialTypeOk

`func (o *OpenShellGatewayServiceAccountListItem) GetCredentialTypeOk() (*string, bool)`

GetCredentialTypeOk returns a tuple with the CredentialType field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCredentialType

`func (o *OpenShellGatewayServiceAccountListItem) SetCredentialType(v string)`

SetCredentialType sets CredentialType field to given value.


### GetRole

`func (o *OpenShellGatewayServiceAccountListItem) GetRole() OpenShellGatewayServiceAccountRole`

GetRole returns the Role field if non-nil, zero value otherwise.

### GetRoleOk

`func (o *OpenShellGatewayServiceAccountListItem) GetRoleOk() (*OpenShellGatewayServiceAccountRole, bool)`

GetRoleOk returns a tuple with the Role field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRole

`func (o *OpenShellGatewayServiceAccountListItem) SetRole(v OpenShellGatewayServiceAccountRole)`

SetRole sets Role field to given value.


### GetStatus

`func (o *OpenShellGatewayServiceAccountListItem) GetStatus() OpenShellGatewayServiceAccountStatus`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *OpenShellGatewayServiceAccountListItem) GetStatusOk() (*OpenShellGatewayServiceAccountStatus, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *OpenShellGatewayServiceAccountListItem) SetStatus(v OpenShellGatewayServiceAccountStatus)`

SetStatus sets Status field to given value.


### GetCreatedByUserId

`func (o *OpenShellGatewayServiceAccountListItem) GetCreatedByUserId() string`

GetCreatedByUserId returns the CreatedByUserId field if non-nil, zero value otherwise.

### GetCreatedByUserIdOk

`func (o *OpenShellGatewayServiceAccountListItem) GetCreatedByUserIdOk() (*string, bool)`

GetCreatedByUserIdOk returns a tuple with the CreatedByUserId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedByUserId

`func (o *OpenShellGatewayServiceAccountListItem) SetCreatedByUserId(v string)`

SetCreatedByUserId sets CreatedByUserId field to given value.


### GetClientId

`func (o *OpenShellGatewayServiceAccountListItem) GetClientId() string`

GetClientId returns the ClientId field if non-nil, zero value otherwise.

### GetClientIdOk

`func (o *OpenShellGatewayServiceAccountListItem) GetClientIdOk() (*string, bool)`

GetClientIdOk returns a tuple with the ClientId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetClientId

`func (o *OpenShellGatewayServiceAccountListItem) SetClientId(v string)`

SetClientId sets ClientId field to given value.


### GetSubject

`func (o *OpenShellGatewayServiceAccountListItem) GetSubject() string`

GetSubject returns the Subject field if non-nil, zero value otherwise.

### GetSubjectOk

`func (o *OpenShellGatewayServiceAccountListItem) GetSubjectOk() (*string, bool)`

GetSubjectOk returns a tuple with the Subject field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSubject

`func (o *OpenShellGatewayServiceAccountListItem) SetSubject(v string)`

SetSubject sets Subject field to given value.


### GetExpiresAt

`func (o *OpenShellGatewayServiceAccountListItem) GetExpiresAt() time.Time`

GetExpiresAt returns the ExpiresAt field if non-nil, zero value otherwise.

### GetExpiresAtOk

`func (o *OpenShellGatewayServiceAccountListItem) GetExpiresAtOk() (*time.Time, bool)`

GetExpiresAtOk returns a tuple with the ExpiresAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExpiresAt

`func (o *OpenShellGatewayServiceAccountListItem) SetExpiresAt(v time.Time)`

SetExpiresAt sets ExpiresAt field to given value.


### GetRevokedAt

`func (o *OpenShellGatewayServiceAccountListItem) GetRevokedAt() time.Time`

GetRevokedAt returns the RevokedAt field if non-nil, zero value otherwise.

### GetRevokedAtOk

`func (o *OpenShellGatewayServiceAccountListItem) GetRevokedAtOk() (*time.Time, bool)`

GetRevokedAtOk returns a tuple with the RevokedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRevokedAt

`func (o *OpenShellGatewayServiceAccountListItem) SetRevokedAt(v time.Time)`

SetRevokedAt sets RevokedAt field to given value.

### HasRevokedAt

`func (o *OpenShellGatewayServiceAccountListItem) HasRevokedAt() bool`

HasRevokedAt returns a boolean if a field has been set.

### SetRevokedAtNil

`func (o *OpenShellGatewayServiceAccountListItem) SetRevokedAtNil(b bool)`

 SetRevokedAtNil sets the value for RevokedAt to be an explicit nil

### UnsetRevokedAt
`func (o *OpenShellGatewayServiceAccountListItem) UnsetRevokedAt()`

UnsetRevokedAt ensures that no value is present for RevokedAt, not even an explicit nil
### GetLastError

`func (o *OpenShellGatewayServiceAccountListItem) GetLastError() string`

GetLastError returns the LastError field if non-nil, zero value otherwise.

### GetLastErrorOk

`func (o *OpenShellGatewayServiceAccountListItem) GetLastErrorOk() (*string, bool)`

GetLastErrorOk returns a tuple with the LastError field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLastError

`func (o *OpenShellGatewayServiceAccountListItem) SetLastError(v string)`

SetLastError sets LastError field to given value.

### HasLastError

`func (o *OpenShellGatewayServiceAccountListItem) HasLastError() bool`

HasLastError returns a boolean if a field has been set.

### SetLastErrorNil

`func (o *OpenShellGatewayServiceAccountListItem) SetLastErrorNil(b bool)`

 SetLastErrorNil sets the value for LastError to be an explicit nil

### UnsetLastError
`func (o *OpenShellGatewayServiceAccountListItem) UnsetLastError()`

UnsetLastError ensures that no value is present for LastError, not even an explicit nil
### GetCreatedAt

`func (o *OpenShellGatewayServiceAccountListItem) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *OpenShellGatewayServiceAccountListItem) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *OpenShellGatewayServiceAccountListItem) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.


### GetUpdatedAt

`func (o *OpenShellGatewayServiceAccountListItem) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *OpenShellGatewayServiceAccountListItem) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *OpenShellGatewayServiceAccountListItem) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


