# Gateway

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | Pointer to **string** |  | [optional] 
**Kind** | Pointer to **string** |  | [optional] 
**Href** | Pointer to **string** |  | [optional] 
**CreatedAt** | Pointer to **time.Time** |  | [optional] 
**UpdatedAt** | Pointer to **time.Time** |  | [optional] 
**Name** | **string** |  | 
**FleetId** | **string** |  | 
**ClusterId** | **string** |  | 
**ReleaseId** | **string** |  | 
**DatabaseId** | **string** | Server-assigned ManagedDatabase identifier; client-supplied values are ignored |
**Namespace** | **string** | API-assigned Kubernetes namespace derived from the Gateway identifier | [readonly] 
**ExternalDns** | Pointer to **string** |  | [optional] 
**TlsMode** | Pointer to **string** |  | [optional] 
**ServiceType** | Pointer to **string** |  | [optional] 
**Status** | Pointer to **string** |  | [optional] 
**Phase** | Pointer to **string** |  | [optional] 
**Image** | Pointer to **string** | Container image for the gateway deployment | [optional] 
**SupervisorImage** | Pointer to **string** | Container image for the supervisor sidecar | [optional] 
**ServerDnsNames** | Pointer to **[]string** | DNS names for TLS certificate SANs | [optional] 
**RouteAddress** | Pointer to **string** | External route address populated by the control plane | [optional] [readonly] 
**ConsoleAddress** | Pointer to **string** | Web console address populated by the control plane | [optional] [readonly] 
**Oidc** | Pointer to **string** | JSON-encoded OIDC authentication configuration (auto-populated by Keycloak provisioning) | [optional] [readonly] 
**Route** | Pointer to **string** | JSON-encoded route configuration | [optional] 
**CredentialDriver** | Pointer to **string** | JSON-encoded credential storage driver configuration | [optional] 
**ActiveSandboxCount** | Pointer to **int32** | Number of active (Running or Pending) agent sandboxes observed in the gateway namespace by the control plane | [optional] [readonly] 
**CreatedBy** | Pointer to **string** | Username of the user who provisioned this gateway, resolved from RBAC role bindings | [optional] [readonly] 

## Methods

### NewGateway

`func NewGateway(name string, fleetId string, clusterId string, releaseId string, databaseId string, namespace string, ) *Gateway`

NewGateway instantiates a new Gateway object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewGatewayWithDefaults

`func NewGatewayWithDefaults() *Gateway`

NewGatewayWithDefaults instantiates a new Gateway object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *Gateway) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *Gateway) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *Gateway) SetId(v string)`

SetId sets Id field to given value.

### HasId

`func (o *Gateway) HasId() bool`

HasId returns a boolean if a field has been set.

### GetKind

`func (o *Gateway) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *Gateway) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *Gateway) SetKind(v string)`

SetKind sets Kind field to given value.

### HasKind

`func (o *Gateway) HasKind() bool`

HasKind returns a boolean if a field has been set.

### GetHref

`func (o *Gateway) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *Gateway) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *Gateway) SetHref(v string)`

SetHref sets Href field to given value.

### HasHref

`func (o *Gateway) HasHref() bool`

HasHref returns a boolean if a field has been set.

### GetCreatedAt

`func (o *Gateway) GetCreatedAt() time.Time`

GetCreatedAt returns the CreatedAt field if non-nil, zero value otherwise.

### GetCreatedAtOk

`func (o *Gateway) GetCreatedAtOk() (*time.Time, bool)`

GetCreatedAtOk returns a tuple with the CreatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedAt

`func (o *Gateway) SetCreatedAt(v time.Time)`

SetCreatedAt sets CreatedAt field to given value.

### HasCreatedAt

`func (o *Gateway) HasCreatedAt() bool`

HasCreatedAt returns a boolean if a field has been set.

### GetUpdatedAt

`func (o *Gateway) GetUpdatedAt() time.Time`

GetUpdatedAt returns the UpdatedAt field if non-nil, zero value otherwise.

### GetUpdatedAtOk

`func (o *Gateway) GetUpdatedAtOk() (*time.Time, bool)`

GetUpdatedAtOk returns a tuple with the UpdatedAt field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetUpdatedAt

`func (o *Gateway) SetUpdatedAt(v time.Time)`

SetUpdatedAt sets UpdatedAt field to given value.

### HasUpdatedAt

`func (o *Gateway) HasUpdatedAt() bool`

HasUpdatedAt returns a boolean if a field has been set.

### GetName

`func (o *Gateway) GetName() string`

GetName returns the Name field if non-nil, zero value otherwise.

### GetNameOk

`func (o *Gateway) GetNameOk() (*string, bool)`

GetNameOk returns a tuple with the Name field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetName

`func (o *Gateway) SetName(v string)`

SetName sets Name field to given value.


### GetFleetId

`func (o *Gateway) GetFleetId() string`

GetFleetId returns the FleetId field if non-nil, zero value otherwise.

### GetFleetIdOk

`func (o *Gateway) GetFleetIdOk() (*string, bool)`

GetFleetIdOk returns a tuple with the FleetId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetFleetId

`func (o *Gateway) SetFleetId(v string)`

SetFleetId sets FleetId field to given value.


### GetClusterId

`func (o *Gateway) GetClusterId() string`

GetClusterId returns the ClusterId field if non-nil, zero value otherwise.

### GetClusterIdOk

`func (o *Gateway) GetClusterIdOk() (*string, bool)`

GetClusterIdOk returns a tuple with the ClusterId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetClusterId

`func (o *Gateway) SetClusterId(v string)`

SetClusterId sets ClusterId field to given value.


### GetReleaseId

`func (o *Gateway) GetReleaseId() string`

GetReleaseId returns the ReleaseId field if non-nil, zero value otherwise.

### GetReleaseIdOk

`func (o *Gateway) GetReleaseIdOk() (*string, bool)`

GetReleaseIdOk returns a tuple with the ReleaseId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetReleaseId

`func (o *Gateway) SetReleaseId(v string)`

SetReleaseId sets ReleaseId field to given value.


### GetDatabaseId

`func (o *Gateway) GetDatabaseId() string`

GetDatabaseId returns the DatabaseId field if non-nil, zero value otherwise.

### GetDatabaseIdOk

`func (o *Gateway) GetDatabaseIdOk() (*string, bool)`

GetDatabaseIdOk returns a tuple with the DatabaseId field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetDatabaseId

`func (o *Gateway) SetDatabaseId(v string)`

SetDatabaseId sets DatabaseId field to given value.


### GetNamespace

`func (o *Gateway) GetNamespace() string`

GetNamespace returns the Namespace field if non-nil, zero value otherwise.

### GetNamespaceOk

`func (o *Gateway) GetNamespaceOk() (*string, bool)`

GetNamespaceOk returns a tuple with the Namespace field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetNamespace

`func (o *Gateway) SetNamespace(v string)`

SetNamespace sets Namespace field to given value.


### GetExternalDns

`func (o *Gateway) GetExternalDns() string`

GetExternalDns returns the ExternalDns field if non-nil, zero value otherwise.

### GetExternalDnsOk

`func (o *Gateway) GetExternalDnsOk() (*string, bool)`

GetExternalDnsOk returns a tuple with the ExternalDns field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetExternalDns

`func (o *Gateway) SetExternalDns(v string)`

SetExternalDns sets ExternalDns field to given value.

### HasExternalDns

`func (o *Gateway) HasExternalDns() bool`

HasExternalDns returns a boolean if a field has been set.

### GetTlsMode

`func (o *Gateway) GetTlsMode() string`

GetTlsMode returns the TlsMode field if non-nil, zero value otherwise.

### GetTlsModeOk

`func (o *Gateway) GetTlsModeOk() (*string, bool)`

GetTlsModeOk returns a tuple with the TlsMode field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetTlsMode

`func (o *Gateway) SetTlsMode(v string)`

SetTlsMode sets TlsMode field to given value.

### HasTlsMode

`func (o *Gateway) HasTlsMode() bool`

HasTlsMode returns a boolean if a field has been set.

### GetServiceType

`func (o *Gateway) GetServiceType() string`

GetServiceType returns the ServiceType field if non-nil, zero value otherwise.

### GetServiceTypeOk

`func (o *Gateway) GetServiceTypeOk() (*string, bool)`

GetServiceTypeOk returns a tuple with the ServiceType field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetServiceType

`func (o *Gateway) SetServiceType(v string)`

SetServiceType sets ServiceType field to given value.

### HasServiceType

`func (o *Gateway) HasServiceType() bool`

HasServiceType returns a boolean if a field has been set.

### GetStatus

`func (o *Gateway) GetStatus() string`

GetStatus returns the Status field if non-nil, zero value otherwise.

### GetStatusOk

`func (o *Gateway) GetStatusOk() (*string, bool)`

GetStatusOk returns a tuple with the Status field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetStatus

`func (o *Gateway) SetStatus(v string)`

SetStatus sets Status field to given value.

### HasStatus

`func (o *Gateway) HasStatus() bool`

HasStatus returns a boolean if a field has been set.

### GetPhase

`func (o *Gateway) GetPhase() string`

GetPhase returns the Phase field if non-nil, zero value otherwise.

### GetPhaseOk

`func (o *Gateway) GetPhaseOk() (*string, bool)`

GetPhaseOk returns a tuple with the Phase field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetPhase

`func (o *Gateway) SetPhase(v string)`

SetPhase sets Phase field to given value.

### HasPhase

`func (o *Gateway) HasPhase() bool`

HasPhase returns a boolean if a field has been set.

### GetImage

`func (o *Gateway) GetImage() string`

GetImage returns the Image field if non-nil, zero value otherwise.

### GetImageOk

`func (o *Gateway) GetImageOk() (*string, bool)`

GetImageOk returns a tuple with the Image field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetImage

`func (o *Gateway) SetImage(v string)`

SetImage sets Image field to given value.

### HasImage

`func (o *Gateway) HasImage() bool`

HasImage returns a boolean if a field has been set.

### GetSupervisorImage

`func (o *Gateway) GetSupervisorImage() string`

GetSupervisorImage returns the SupervisorImage field if non-nil, zero value otherwise.

### GetSupervisorImageOk

`func (o *Gateway) GetSupervisorImageOk() (*string, bool)`

GetSupervisorImageOk returns a tuple with the SupervisorImage field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetSupervisorImage

`func (o *Gateway) SetSupervisorImage(v string)`

SetSupervisorImage sets SupervisorImage field to given value.

### HasSupervisorImage

`func (o *Gateway) HasSupervisorImage() bool`

HasSupervisorImage returns a boolean if a field has been set.

### GetServerDnsNames

`func (o *Gateway) GetServerDnsNames() []string`

GetServerDnsNames returns the ServerDnsNames field if non-nil, zero value otherwise.

### GetServerDnsNamesOk

`func (o *Gateway) GetServerDnsNamesOk() (*[]string, bool)`

GetServerDnsNamesOk returns a tuple with the ServerDnsNames field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetServerDnsNames

`func (o *Gateway) SetServerDnsNames(v []string)`

SetServerDnsNames sets ServerDnsNames field to given value.

### HasServerDnsNames

`func (o *Gateway) HasServerDnsNames() bool`

HasServerDnsNames returns a boolean if a field has been set.

### GetRouteAddress

`func (o *Gateway) GetRouteAddress() string`

GetRouteAddress returns the RouteAddress field if non-nil, zero value otherwise.

### GetRouteAddressOk

`func (o *Gateway) GetRouteAddressOk() (*string, bool)`

GetRouteAddressOk returns a tuple with the RouteAddress field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRouteAddress

`func (o *Gateway) SetRouteAddress(v string)`

SetRouteAddress sets RouteAddress field to given value.

### HasRouteAddress

`func (o *Gateway) HasRouteAddress() bool`

HasRouteAddress returns a boolean if a field has been set.

### GetConsoleAddress

`func (o *Gateway) GetConsoleAddress() string`

GetConsoleAddress returns the ConsoleAddress field if non-nil, zero value otherwise.

### GetConsoleAddressOk

`func (o *Gateway) GetConsoleAddressOk() (*string, bool)`

GetConsoleAddressOk returns a tuple with the ConsoleAddress field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetConsoleAddress

`func (o *Gateway) SetConsoleAddress(v string)`

SetConsoleAddress sets ConsoleAddress field to given value.

### HasConsoleAddress

`func (o *Gateway) HasConsoleAddress() bool`

HasConsoleAddress returns a boolean if a field has been set.

### GetOidc

`func (o *Gateway) GetOidc() string`

GetOidc returns the Oidc field if non-nil, zero value otherwise.

### GetOidcOk

`func (o *Gateway) GetOidcOk() (*string, bool)`

GetOidcOk returns a tuple with the Oidc field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetOidc

`func (o *Gateway) SetOidc(v string)`

SetOidc sets Oidc field to given value.

### HasOidc

`func (o *Gateway) HasOidc() bool`

HasOidc returns a boolean if a field has been set.

### GetRoute

`func (o *Gateway) GetRoute() string`

GetRoute returns the Route field if non-nil, zero value otherwise.

### GetRouteOk

`func (o *Gateway) GetRouteOk() (*string, bool)`

GetRouteOk returns a tuple with the Route field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetRoute

`func (o *Gateway) SetRoute(v string)`

SetRoute sets Route field to given value.

### HasRoute

`func (o *Gateway) HasRoute() bool`

HasRoute returns a boolean if a field has been set.

### GetCredentialDriver

`func (o *Gateway) GetCredentialDriver() string`

GetCredentialDriver returns the CredentialDriver field if non-nil, zero value otherwise.

### GetCredentialDriverOk

`func (o *Gateway) GetCredentialDriverOk() (*string, bool)`

GetCredentialDriverOk returns a tuple with the CredentialDriver field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCredentialDriver

`func (o *Gateway) SetCredentialDriver(v string)`

SetCredentialDriver sets CredentialDriver field to given value.

### HasCredentialDriver

`func (o *Gateway) HasCredentialDriver() bool`

HasCredentialDriver returns a boolean if a field has been set.

### GetActiveSandboxCount

`func (o *Gateway) GetActiveSandboxCount() int32`

GetActiveSandboxCount returns the ActiveSandboxCount field if non-nil, zero value otherwise.

### GetActiveSandboxCountOk

`func (o *Gateway) GetActiveSandboxCountOk() (*int32, bool)`

GetActiveSandboxCountOk returns a tuple with the ActiveSandboxCount field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetActiveSandboxCount

`func (o *Gateway) SetActiveSandboxCount(v int32)`

SetActiveSandboxCount sets ActiveSandboxCount field to given value.

### HasActiveSandboxCount

`func (o *Gateway) HasActiveSandboxCount() bool`

HasActiveSandboxCount returns a boolean if a field has been set.

### GetCreatedBy

`func (o *Gateway) GetCreatedBy() string`

GetCreatedBy returns the CreatedBy field if non-nil, zero value otherwise.

### GetCreatedByOk

`func (o *Gateway) GetCreatedByOk() (*string, bool)`

GetCreatedByOk returns a tuple with the CreatedBy field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetCreatedBy

`func (o *Gateway) SetCreatedBy(v string)`

SetCreatedBy sets CreatedBy field to given value.

### HasCreatedBy

`func (o *Gateway) HasCreatedBy() bool`

HasCreatedBy returns a boolean if a field has been set.


[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


