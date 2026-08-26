# OpenShellGatewayServiceAccountGetResponse

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
**Connection** | [**OpenShellGatewayServiceAccountConnection**](OpenShellGatewayServiceAccountConnection.md) |  | 

## Methods

### NewOpenShellGatewayServiceAccountGetResponse

`func NewOpenShellGatewayServiceAccountGetResponse(id string, gatewayId string, name string, credentialType string, role OpenShellGatewayServiceAccountRole, status OpenShellGatewayServiceAccountStatus, createdByUserId string, clientId string, subject string, expiresAt time.Time, createdAt time.Time, updatedAt time.Time, connection OpenShellGatewayServiceAccountConnection, ) *OpenShellGatewayServiceAccountGetResponse`

NewOpenShellGatewayServiceAccountGetResponse instantiates a new OpenShellGatewayServiceAccountGetResponse object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewOpenShellGatewayServiceAccountGetResponseWithDefaults

`func NewOpenShellGatewayServiceAccountGetResponseWithDefaults() *OpenShellGatewayServiceAccountGetResponse`

NewOpenShellGatewayServiceAccountGetResponseWithDefaults instantiates a new OpenShellGatewayServiceAccountGetResponse object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *OpenShellGatewayServiceAccountGetResponse) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *OpenShellGatewayServiceAccountGetResponse) SetId(v string)`

SetId sets Id field to given value.


### GetGatewayId

`func (o *OpenShellGatewayServiceAccountGetResponse) GetGatewayId() string`

GetGatewayId returns the GatewayId field if non-nil, zero value otherwise.

### GetGatewayIdOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetGatewayIdOk() (*string, bool)`

GetGatewayIdOk returns a tuple with the GatewayId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetGatewayId

`func (o *OpenShellGatewayServiceAccountGetResponse) SetGatewayId(v string)`

SetGatewayId sets GatewayId field to given value.


### GetName

`func (o *OpenShellGatewayServiceAccountGetResponse) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *OpenShellGatewayServiceAccountGetResponse) SetName(v string)`

SetName sets Name field to given value.


### GetDescription

`func (o *OpenShellGatewayServiceAccountGetResponse) GetDescription() string`

GetDescription returns the Description field if non-nil, zero value otherwise.

### GetDescriptionOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetDescriptionOk() (*string, bool)`

GetDescriptionOk returns a tuple with the Description field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDescription

`func (o *OpenShellGatewayServiceAccountGetResponse) SetDescription(v string)`

SetDescription sets Description field to given value.

### HasDescription

`func (o *OpenShellGatewayServiceAccountGetResponse) HasDescription() bool`

HasDescription returns a boolean if a field has been set.

### SetDescriptionNil

`func (o *OpenShellGatewayServiceAccountGetResponse) SetDescriptionNil(b bool)`

 SetDescriptionNil sets the value for Description to be an explicit nil

### UnsetDescription
`func (o *OpenShellGatewayServiceAccountGetResponse) UnsetDescription()`

UnsetDescription ensures that no value is present for Description, not even an explicit nil
### GetCredentialType

`func (o *OpenShellGatewayServiceAccountGetResponse) GetCredentialType() string`

GetCredentialType returns the CredentialType field if non-nil, zero value otherwise.

### GetCredentialTypeOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetCredentialTypeOk() (*string, bool)`

GetCredentialTypeOk returns a tuple with the CredentialType field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCredentialType

`func (o *OpenShellGatewayServiceAccountGetResponse) SetCredentialType(v string)`

SetCredentialType sets CredentialType field to given value.


### GetRole

`func (o *OpenShellGatewayServiceAccountGetResponse) GetRole() OpenShellGatewayServiceAccountRole`

GetRole returns the Role field if non-nil, zero value otherwise.

### GetRoleOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetRoleOk() (*OpenShellGatewayServiceAccountRole, bool)`

GetRoleOk returns a tuple with the Role field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRole

`func (o *OpenShellGatewayServiceAccountGetResponse) SetRole(v OpenShellGatewayServiceAccountRole)`

SetRole sets Role field to given value.


### GetStatus

`func (o *OpenShellGatewayServiceAccountGetResponse) GetStatus() OpenShellGatewayServiceAccountStatus`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetStatusOk() (*OpenShellGatewayServiceAccountStatus, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *OpenShellGatewayServiceAccountGetResponse) SetStatus(v OpenShellGatewayServiceAccountStatus)`

SetStatus sets Status field to given value.


### GetCreatedByUserId

`func (o *OpenShellGatewayServiceAccountGetResponse) GetCreatedByUserId() string`

GetCreatedByUserId returns the CreatedByUserId field if non-nil, zero value otherwise.

### GetCreatedByUserIdOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetCreatedByUserIdOk() (*string, bool)`

GetCreatedByUserIdOk returns a tuple with the CreatedByUserId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedByUserId

`func (o *OpenShellGatewayServiceAccountGetResponse) SetCreatedByUserId(v string)`

SetCreatedByUserId sets CreatedByUserId field to given value.


### GetClientId

`func (o *OpenShellGatewayServiceAccountGetResponse) GetClientId() string`

GetClientId returns the ClientId field if non-nil, zero value otherwise.

### GetClientIdOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetClientIdOk() (*string, bool)`

GetClientIdOk returns a tuple with the ClientId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetClientId

`func (o *OpenShellGatewayServiceAccountGetResponse) SetClientId(v string)`

SetClientId sets ClientId field to given value.


### GetSubject

`func (o *OpenShellGatewayServiceAccountGetResponse) GetSubject() string`

GetSubject returns the Subject field if non-nil, zero value otherwise.

### GetSubjectOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetSubjectOk() (*string, bool)`

GetSubjectOk returns a tuple with the Subject field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSubject

`func (o *OpenShellGatewayServiceAccountGetResponse) SetSubject(v string)`

SetSubject sets Subject field to given value.


### GetExpiresAt

`func (o *OpenShellGatewayServiceAccountGetResponse) GetExpiresAt() time.Time`

GetExpiresAt returns the ExpiresAt field if non-nil, zero value otherwise.

### GetExpiresAtOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetExpiresAtOk() (*time.Time, bool)`

GetExpiresAtOk returns a tuple with the ExpiresAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExpiresAt

`func (o *OpenShellGatewayServiceAccountGetResponse) SetExpiresAt(v time.Time)`

SetExpiresAt sets ExpiresAt field to given value.


### GetRevokedAt

`func (o *OpenShellGatewayServiceAccountGetResponse) GetRevokedAt() time.Time`

GetRevokedAt returns the RevokedAt field if non-nil, zero value otherwise.

### GetRevokedAtOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetRevokedAtOk() (*time.Time, bool)`

GetRevokedAtOk returns a tuple with the RevokedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRevokedAt

`func (o *OpenShellGatewayServiceAccountGetResponse) SetRevokedAt(v time.Time)`

SetRevokedAt sets RevokedAt field to given value.

### HasRevokedAt

`func (o *OpenShellGatewayServiceAccountGetResponse) HasRevokedAt() bool`

HasRevokedAt returns a boolean if a field has been set.

### SetRevokedAtNil

`func (o *OpenShellGatewayServiceAccountGetResponse) SetRevokedAtNil(b bool)`

 SetRevokedAtNil sets the value for RevokedAt to be an explicit nil

### UnsetRevokedAt
`func (o *OpenShellGatewayServiceAccountGetResponse) UnsetRevokedAt()`

UnsetRevokedAt ensures that no value is present for RevokedAt, not even an explicit nil
### GetLastError

`func (o *OpenShellGatewayServiceAccountGetResponse) GetLastError() string`

GetLastError returns the LastError field if non-nil, zero value otherwise.

### GetLastErrorOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetLastErrorOk() (*string, bool)`

GetLastErrorOk returns a tuple with the LastError field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetLastError

`func (o *OpenShellGatewayServiceAccountGetResponse) SetLastError(v string)`

SetLastError sets LastError field to given value.

### HasLastError

`func (o *OpenShellGatewayServiceAccountGetResponse) HasLastError() bool`

HasLastError returns a boolean if a field has been set.

### SetLastErrorNil

`func (o *OpenShellGatewayServiceAccountGetResponse) SetLastErrorNil(b bool)`

 SetLastErrorNil sets the value for LastError to be an explicit nil

### UnsetLastError
`func (o *OpenShellGatewayServiceAccountGetResponse) UnsetLastError()`

UnsetLastError ensures that no value is present for LastError, not even an explicit nil
### GetCreatedAt

`func (o *OpenShellGatewayServiceAccountGetResponse) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *OpenShellGatewayServiceAccountGetResponse) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.


### GetUpdatedAt

`func (o *OpenShellGatewayServiceAccountGetResponse) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *OpenShellGatewayServiceAccountGetResponse) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.


### GetConnection

`func (o *OpenShellGatewayServiceAccountGetResponse) GetConnection() OpenShellGatewayServiceAccountConnection`

GetConnection returns the Connection field if non-nil, zero value otherwise.

### GetConnectionOk

`func (o *OpenShellGatewayServiceAccountGetResponse) GetConnectionOk() (*OpenShellGatewayServiceAccountConnection, bool)`

GetConnectionOk returns a tuple with the Connection field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetConnection

`func (o *OpenShellGatewayServiceAccountGetResponse) SetConnection(v OpenShellGatewayServiceAccountConnection)`

SetConnection sets Connection field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


