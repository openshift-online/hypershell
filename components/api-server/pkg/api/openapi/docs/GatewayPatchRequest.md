# GatewayPatchRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | Pointer to **string** |  | [optional] 
**ClusterId** | Pointer to **string** |  | [optional] 
**ReleaseId** | Pointer to **string** |  | [optional] 
**ProfileId** | Pointer to **string** | Reassign the gateway to a different GatewayProfile; cannot be set to null or empty (HTTP 400) and the target profile must exist | [optional] 
**DatabaseId** | Pointer to **string** | Server-owned placement field; values supplied through PATCH are ignored | [optional] 
**ExternalDns** | Pointer to **string** |  | [optional] 
**TlsMode** | Pointer to **string** |  | [optional] 
**ServiceType** | Pointer to **string** |  | [optional] 
**Status** | Pointer to **string** |  | [optional] 
**Phase** | Pointer to **string** |  | [optional] 
**Image** | Pointer to **string** |  | [optional] 
**SupervisorImage** | Pointer to **string** |  | [optional] 
**ServerDnsNames** | Pointer to **[]string** |  | [optional] 
**RouteAddress** | Pointer to **string** |  | [optional] 
**Oidc** | Pointer to **string** |  | [optional] 
**Route** | Pointer to **string** |  | [optional] 
**CredentialDriver** | Pointer to **string** |  | [optional] 

## Methods

### NewGatewayPatchRequest

`func NewGatewayPatchRequest() *GatewayPatchRequest`

NewGatewayPatchRequest instantiates a new GatewayPatchRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayPatchRequestWithDefaults

`func NewGatewayPatchRequestWithDefaults() *GatewayPatchRequest`

NewGatewayPatchRequestWithDefaults instantiates a new GatewayPatchRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *GatewayPatchRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *GatewayPatchRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *GatewayPatchRequest) SetName(v string)`

SetName sets Name field to given value.

### HasName

`func (o *GatewayPatchRequest) HasName() bool`

HasName returns a boolean if a field has been set.

### GetClusterId

`func (o *GatewayPatchRequest) GetClusterId() string`

GetClusterId returns the ClusterId field if non-nil, zero value otherwise.

### GetClusterIdOk

`func (o *GatewayPatchRequest) GetClusterIdOk() (*string, bool)`

GetClusterIdOk returns a tuple with the ClusterId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetClusterId

`func (o *GatewayPatchRequest) SetClusterId(v string)`

SetClusterId sets ClusterId field to given value.

### HasClusterId

`func (o *GatewayPatchRequest) HasClusterId() bool`

HasClusterId returns a boolean if a field has been set.

### GetReleaseId

`func (o *GatewayPatchRequest) GetReleaseId() string`

GetReleaseId returns the ReleaseId field if non-nil, zero value otherwise.

### GetReleaseIdOk

`func (o *GatewayPatchRequest) GetReleaseIdOk() (*string, bool)`

GetReleaseIdOk returns a tuple with the ReleaseId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetReleaseId

`func (o *GatewayPatchRequest) SetReleaseId(v string)`

SetReleaseId sets ReleaseId field to given value.

### HasReleaseId

`func (o *GatewayPatchRequest) HasReleaseId() bool`

HasReleaseId returns a boolean if a field has been set.

### GetProfileId

`func (o *GatewayPatchRequest) GetProfileId() string`

GetProfileId returns the ProfileId field if non-nil, zero value otherwise.

### GetProfileIdOk

`func (o *GatewayPatchRequest) GetProfileIdOk() (*string, bool)`

GetProfileIdOk returns a tuple with the ProfileId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetProfileId

`func (o *GatewayPatchRequest) SetProfileId(v string)`

SetProfileId sets ProfileId field to given value.

### HasProfileId

`func (o *GatewayPatchRequest) HasProfileId() bool`

HasProfileId returns a boolean if a field has been set.

### GetDatabaseId

`func (o *GatewayPatchRequest) GetDatabaseId() string`

GetDatabaseId returns the DatabaseId field if non-nil, zero value otherwise.

### GetDatabaseIdOk

`func (o *GatewayPatchRequest) GetDatabaseIdOk() (*string, bool)`

GetDatabaseIdOk returns a tuple with the DatabaseId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDatabaseId

`func (o *GatewayPatchRequest) SetDatabaseId(v string)`

SetDatabaseId sets DatabaseId field to given value.

### HasDatabaseId

`func (o *GatewayPatchRequest) HasDatabaseId() bool`

HasDatabaseId returns a boolean if a field has been set.

### GetExternalDns

`func (o *GatewayPatchRequest) GetExternalDns() string`

GetExternalDns returns the ExternalDns field if non-nil, zero value otherwise.

### GetExternalDnsOk

`func (o *GatewayPatchRequest) GetExternalDnsOk() (*string, bool)`

GetExternalDnsOk returns a tuple with the ExternalDns field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExternalDns

`func (o *GatewayPatchRequest) SetExternalDns(v string)`

SetExternalDns sets ExternalDns field to given value.

### HasExternalDns

`func (o *GatewayPatchRequest) HasExternalDns() bool`

HasExternalDns returns a boolean if a field has been set.

### GetTlsMode

`func (o *GatewayPatchRequest) GetTlsMode() string`

GetTlsMode returns the TlsMode field if non-nil, zero value otherwise.

### GetTlsModeOk

`func (o *GatewayPatchRequest) GetTlsModeOk() (*string, bool)`

GetTlsModeOk returns a tuple with the TlsMode field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTlsMode

`func (o *GatewayPatchRequest) SetTlsMode(v string)`

SetTlsMode sets TlsMode field to given value.

### HasTlsMode

`func (o *GatewayPatchRequest) HasTlsMode() bool`

HasTlsMode returns a boolean if a field has been set.

### GetServiceType

`func (o *GatewayPatchRequest) GetServiceType() string`

GetServiceType returns the ServiceType field if non-nil, zero value otherwise.

### GetServiceTypeOk

`func (o *GatewayPatchRequest) GetServiceTypeOk() (*string, bool)`

GetServiceTypeOk returns a tuple with the ServiceType field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetServiceType

`func (o *GatewayPatchRequest) SetServiceType(v string)`

SetServiceType sets ServiceType field to given value.

### HasServiceType

`func (o *GatewayPatchRequest) HasServiceType() bool`

HasServiceType returns a boolean if a field has been set.

### GetStatus

`func (o *GatewayPatchRequest) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *GatewayPatchRequest) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *GatewayPatchRequest) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *GatewayPatchRequest) HasStatus() bool`

HasStatus returns a boolean if a field has been set.

### GetPhase

`func (o *GatewayPatchRequest) GetPhase() string`

GetPhase returns the Phase field if non-nil, zero value otherwise.

### GetPhaseOk

`func (o *GatewayPatchRequest) GetPhaseOk() (*string, bool)`

GetPhaseOk returns a tuple with the Phase field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPhase

`func (o *GatewayPatchRequest) SetPhase(v string)`

SetPhase sets Phase field to given value.

### HasPhase

`func (o *GatewayPatchRequest) HasPhase() bool`

HasPhase returns a boolean if a field has been set.

### GetImage

`func (o *GatewayPatchRequest) GetImage() string`

GetImage returns the Image field if non-nil, zero value otherwise.

### GetImageOk

`func (o *GatewayPatchRequest) GetImageOk() (*string, bool)`

GetImageOk returns a tuple with the Image field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetImage

`func (o *GatewayPatchRequest) SetImage(v string)`

SetImage sets Image field to given value.

### HasImage

`func (o *GatewayPatchRequest) HasImage() bool`

HasImage returns a boolean if a field has been set.

### GetSupervisorImage

`func (o *GatewayPatchRequest) GetSupervisorImage() string`

GetSupervisorImage returns the SupervisorImage field if non-nil, zero value otherwise.

### GetSupervisorImageOk

`func (o *GatewayPatchRequest) GetSupervisorImageOk() (*string, bool)`

GetSupervisorImageOk returns a tuple with the SupervisorImage field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSupervisorImage

`func (o *GatewayPatchRequest) SetSupervisorImage(v string)`

SetSupervisorImage sets SupervisorImage field to given value.

### HasSupervisorImage

`func (o *GatewayPatchRequest) HasSupervisorImage() bool`

HasSupervisorImage returns a boolean if a field has been set.

### GetServerDnsNames

`func (o *GatewayPatchRequest) GetServerDnsNames() []string`

GetServerDnsNames returns the ServerDnsNames field if non-nil, zero value otherwise.

### GetServerDnsNamesOk

`func (o *GatewayPatchRequest) GetServerDnsNamesOk() (*[]string, bool)`

GetServerDnsNamesOk returns a tuple with the ServerDnsNames field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetServerDnsNames

`func (o *GatewayPatchRequest) SetServerDnsNames(v []string)`

SetServerDnsNames sets ServerDnsNames field to given value.

### HasServerDnsNames

`func (o *GatewayPatchRequest) HasServerDnsNames() bool`

HasServerDnsNames returns a boolean if a field has been set.

### GetRouteAddress

`func (o *GatewayPatchRequest) GetRouteAddress() string`

GetRouteAddress returns the RouteAddress field if non-nil, zero value otherwise.

### GetRouteAddressOk

`func (o *GatewayPatchRequest) GetRouteAddressOk() (*string, bool)`

GetRouteAddressOk returns a tuple with the RouteAddress field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRouteAddress

`func (o *GatewayPatchRequest) SetRouteAddress(v string)`

SetRouteAddress sets RouteAddress field to given value.

### HasRouteAddress

`func (o *GatewayPatchRequest) HasRouteAddress() bool`

HasRouteAddress returns a boolean if a field has been set.

### GetOidc

`func (o *GatewayPatchRequest) GetOidc() string`

GetOidc returns the Oidc field if non-nil, zero value otherwise.

### GetOidcOk

`func (o *GatewayPatchRequest) GetOidcOk() (*string, bool)`

GetOidcOk returns a tuple with the Oidc field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOidc

`func (o *GatewayPatchRequest) SetOidc(v string)`

SetOidc sets Oidc field to given value.

### HasOidc

`func (o *GatewayPatchRequest) HasOidc() bool`

HasOidc returns a boolean if a field has been set.

### GetRoute

`func (o *GatewayPatchRequest) GetRoute() string`

GetRoute returns the Route field if non-nil, zero value otherwise.

### GetRouteOk

`func (o *GatewayPatchRequest) GetRouteOk() (*string, bool)`

GetRouteOk returns a tuple with the Route field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRoute

`func (o *GatewayPatchRequest) SetRoute(v string)`

SetRoute sets Route field to given value.

### HasRoute

`func (o *GatewayPatchRequest) HasRoute() bool`

HasRoute returns a boolean if a field has been set.

### GetCredentialDriver

`func (o *GatewayPatchRequest) GetCredentialDriver() string`

GetCredentialDriver returns the CredentialDriver field if non-nil, zero value otherwise.

### GetCredentialDriverOk

`func (o *GatewayPatchRequest) GetCredentialDriverOk() (*string, bool)`

GetCredentialDriverOk returns a tuple with the CredentialDriver field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCredentialDriver

`func (o *GatewayPatchRequest) SetCredentialDriver(v string)`

SetCredentialDriver sets CredentialDriver field to given value.

### HasCredentialDriver

`func (o *GatewayPatchRequest) HasCredentialDriver() bool`

HasCredentialDriver returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


