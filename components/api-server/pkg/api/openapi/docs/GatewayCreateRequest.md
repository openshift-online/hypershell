# GatewayCreateRequest

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Name** | **string** |  | 
**FleetId** | **string** |  | 
**ClusterId** | **string** |  | 
**ReleaseId** | **string** |  | 
**DatabaseId** | **string** |  | 
**ExternalDns** | Pointer to **string** |  | [optional] 
**TlsMode** | Pointer to **string** |  | [optional] 
**ServiceType** | Pointer to **string** |  | [optional] 
**Status** | Pointer to **string** |  | [optional] 
**Phase** | Pointer to **string** |  | [optional] 
**Image** | Pointer to **string** | Container image for the gateway deployment | [optional] 
**SupervisorImage** | Pointer to **string** | Container image for the supervisor sidecar | [optional] 
**ServerDnsNames** | Pointer to **[]string** | DNS names for TLS certificate SANs | [optional] 
**Oidc** | Pointer to **string** | JSON-encoded OIDC authentication configuration | [optional] 
**Route** | Pointer to **string** | JSON-encoded route configuration | [optional] 
**CredentialDriver** | Pointer to **string** | JSON-encoded credential storage driver configuration | [optional] 

## Methods

### NewGatewayCreateRequest

`func NewGatewayCreateRequest(name string, fleetId string, clusterId string, releaseId string, databaseId string, ) *GatewayCreateRequest`

NewGatewayCreateRequest instantiates a new GatewayCreateRequest object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayCreateRequestWithDefaults

`func NewGatewayCreateRequestWithDefaults() *GatewayCreateRequest`

NewGatewayCreateRequestWithDefaults instantiates a new GatewayCreateRequest object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetName

`func (o *GatewayCreateRequest) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *GatewayCreateRequest) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *GatewayCreateRequest) SetName(v string)`

SetName sets Name field to given value.


### GetFleetId

`func (o *GatewayCreateRequest) GetFleetId() string`

GetFleetId returns the FleetId field if non-nil, zero value otherwise.

### GetFleetIdOk

`func (o *GatewayCreateRequest) GetFleetIdOk() (*string, bool)`

GetFleetIdOk returns a tuple with the FleetId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetFleetId

`func (o *GatewayCreateRequest) SetFleetId(v string)`

SetFleetId sets FleetId field to given value.


### GetClusterId

`func (o *GatewayCreateRequest) GetClusterId() string`

GetClusterId returns the ClusterId field if non-nil, zero value otherwise.

### GetClusterIdOk

`func (o *GatewayCreateRequest) GetClusterIdOk() (*string, bool)`

GetClusterIdOk returns a tuple with the ClusterId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetClusterId

`func (o *GatewayCreateRequest) SetClusterId(v string)`

SetClusterId sets ClusterId field to given value.


### GetReleaseId

`func (o *GatewayCreateRequest) GetReleaseId() string`

GetReleaseId returns the ReleaseId field if non-nil, zero value otherwise.

### GetReleaseIdOk

`func (o *GatewayCreateRequest) GetReleaseIdOk() (*string, bool)`

GetReleaseIdOk returns a tuple with the ReleaseId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetReleaseId

`func (o *GatewayCreateRequest) SetReleaseId(v string)`

SetReleaseId sets ReleaseId field to given value.


### GetDatabaseId

`func (o *GatewayCreateRequest) GetDatabaseId() string`

GetDatabaseId returns the DatabaseId field if non-nil, zero value otherwise.

### GetDatabaseIdOk

`func (o *GatewayCreateRequest) GetDatabaseIdOk() (*string, bool)`

GetDatabaseIdOk returns a tuple with the DatabaseId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDatabaseId

`func (o *GatewayCreateRequest) SetDatabaseId(v string)`

SetDatabaseId sets DatabaseId field to given value.


### GetExternalDns

`func (o *GatewayCreateRequest) GetExternalDns() string`

GetExternalDns returns the ExternalDns field if non-nil, zero value otherwise.

### GetExternalDnsOk

`func (o *GatewayCreateRequest) GetExternalDnsOk() (*string, bool)`

GetExternalDnsOk returns a tuple with the ExternalDns field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExternalDns

`func (o *GatewayCreateRequest) SetExternalDns(v string)`

SetExternalDns sets ExternalDns field to given value.

### HasExternalDns

`func (o *GatewayCreateRequest) HasExternalDns() bool`

HasExternalDns returns a boolean if a field has been set.

### GetTlsMode

`func (o *GatewayCreateRequest) GetTlsMode() string`

GetTlsMode returns the TlsMode field if non-nil, zero value otherwise.

### GetTlsModeOk

`func (o *GatewayCreateRequest) GetTlsModeOk() (*string, bool)`

GetTlsModeOk returns a tuple with the TlsMode field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTlsMode

`func (o *GatewayCreateRequest) SetTlsMode(v string)`

SetTlsMode sets TlsMode field to given value.

### HasTlsMode

`func (o *GatewayCreateRequest) HasTlsMode() bool`

HasTlsMode returns a boolean if a field has been set.

### GetServiceType

`func (o *GatewayCreateRequest) GetServiceType() string`

GetServiceType returns the ServiceType field if non-nil, zero value otherwise.

### GetServiceTypeOk

`func (o *GatewayCreateRequest) GetServiceTypeOk() (*string, bool)`

GetServiceTypeOk returns a tuple with the ServiceType field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetServiceType

`func (o *GatewayCreateRequest) SetServiceType(v string)`

SetServiceType sets ServiceType field to given value.

### HasServiceType

`func (o *GatewayCreateRequest) HasServiceType() bool`

HasServiceType returns a boolean if a field has been set.

### GetStatus

`func (o *GatewayCreateRequest) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *GatewayCreateRequest) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *GatewayCreateRequest) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *GatewayCreateRequest) HasStatus() bool`

HasStatus returns a boolean if a field has been set.

### GetPhase

`func (o *GatewayCreateRequest) GetPhase() string`

GetPhase returns the Phase field if non-nil, zero value otherwise.

### GetPhaseOk

`func (o *GatewayCreateRequest) GetPhaseOk() (*string, bool)`

GetPhaseOk returns a tuple with the Phase field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPhase

`func (o *GatewayCreateRequest) SetPhase(v string)`

SetPhase sets Phase field to given value.

### HasPhase

`func (o *GatewayCreateRequest) HasPhase() bool`

HasPhase returns a boolean if a field has been set.

### GetImage

`func (o *GatewayCreateRequest) GetImage() string`

GetImage returns the Image field if non-nil, zero value otherwise.

### GetImageOk

`func (o *GatewayCreateRequest) GetImageOk() (*string, bool)`

GetImageOk returns a tuple with the Image field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetImage

`func (o *GatewayCreateRequest) SetImage(v string)`

SetImage sets Image field to given value.

### HasImage

`func (o *GatewayCreateRequest) HasImage() bool`

HasImage returns a boolean if a field has been set.

### GetSupervisorImage

`func (o *GatewayCreateRequest) GetSupervisorImage() string`

GetSupervisorImage returns the SupervisorImage field if non-nil, zero value otherwise.

### GetSupervisorImageOk

`func (o *GatewayCreateRequest) GetSupervisorImageOk() (*string, bool)`

GetSupervisorImageOk returns a tuple with the SupervisorImage field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSupervisorImage

`func (o *GatewayCreateRequest) SetSupervisorImage(v string)`

SetSupervisorImage sets SupervisorImage field to given value.

### HasSupervisorImage

`func (o *GatewayCreateRequest) HasSupervisorImage() bool`

HasSupervisorImage returns a boolean if a field has been set.

### GetServerDnsNames

`func (o *GatewayCreateRequest) GetServerDnsNames() []string`

GetServerDnsNames returns the ServerDnsNames field if non-nil, zero value otherwise.

### GetServerDnsNamesOk

`func (o *GatewayCreateRequest) GetServerDnsNamesOk() (*[]string, bool)`

GetServerDnsNamesOk returns a tuple with the ServerDnsNames field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetServerDnsNames

`func (o *GatewayCreateRequest) SetServerDnsNames(v []string)`

SetServerDnsNames sets ServerDnsNames field to given value.

### HasServerDnsNames

`func (o *GatewayCreateRequest) HasServerDnsNames() bool`

HasServerDnsNames returns a boolean if a field has been set.

### GetOidc

`func (o *GatewayCreateRequest) GetOidc() string`

GetOidc returns the Oidc field if non-nil, zero value otherwise.

### GetOidcOk

`func (o *GatewayCreateRequest) GetOidcOk() (*string, bool)`

GetOidcOk returns a tuple with the Oidc field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOidc

`func (o *GatewayCreateRequest) SetOidc(v string)`

SetOidc sets Oidc field to given value.

### HasOidc

`func (o *GatewayCreateRequest) HasOidc() bool`

HasOidc returns a boolean if a field has been set.

### GetRoute

`func (o *GatewayCreateRequest) GetRoute() string`

GetRoute returns the Route field if non-nil, zero value otherwise.

### GetRouteOk

`func (o *GatewayCreateRequest) GetRouteOk() (*string, bool)`

GetRouteOk returns a tuple with the Route field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRoute

`func (o *GatewayCreateRequest) SetRoute(v string)`

SetRoute sets Route field to given value.

### HasRoute

`func (o *GatewayCreateRequest) HasRoute() bool`

HasRoute returns a boolean if a field has been set.

### GetCredentialDriver

`func (o *GatewayCreateRequest) GetCredentialDriver() string`

GetCredentialDriver returns the CredentialDriver field if non-nil, zero value otherwise.

### GetCredentialDriverOk

`func (o *GatewayCreateRequest) GetCredentialDriverOk() (*string, bool)`

GetCredentialDriverOk returns a tuple with the CredentialDriver field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCredentialDriver

`func (o *GatewayCreateRequest) SetCredentialDriver(v string)`

SetCredentialDriver sets CredentialDriver field to given value.

### HasCredentialDriver

`func (o *GatewayCreateRequest) HasCredentialDriver() bool`

HasCredentialDriver returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


