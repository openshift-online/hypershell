# ServiceMetadata

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**Id** | **string** | Service identifier | 
**Href** | **string** | Metadata request path | 
**Kind** | **string** |  | 
**Version** | **string** | API server image build version | 
**BuildTime** | **string** | Time when the API server binary was built | 

## Methods

### NewServiceMetadata

`func NewServiceMetadata(id string, href string, kind string, version string, buildTime string, ) *ServiceMetadata`

NewServiceMetadata instantiates a new ServiceMetadata object
This constructor will assign default values to properties that have it defined,
and makes sure properties required by API are set, but the set of arguments
will change when the set of required properties is changed

### NewServiceMetadataWithDefaults

`func NewServiceMetadataWithDefaults() *ServiceMetadata`

NewServiceMetadataWithDefaults instantiates a new ServiceMetadata object
This constructor will only assign default values to properties that have it defined,
but it doesn't guarantee that properties required by API are set

### GetId

`func (o *ServiceMetadata) GetId() string`

GetId returns the Id field if non-nil, zero value otherwise.

### GetIdOk

`func (o *ServiceMetadata) GetIdOk() (*string, bool)`

GetIdOk returns a tuple with the Id field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetId

`func (o *ServiceMetadata) SetId(v string)`

SetId sets Id field to given value.


### GetHref

`func (o *ServiceMetadata) GetHref() string`

GetHref returns the Href field if non-nil, zero value otherwise.

### GetHrefOk

`func (o *ServiceMetadata) GetHrefOk() (*string, bool)`

GetHrefOk returns a tuple with the Href field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetHref

`func (o *ServiceMetadata) SetHref(v string)`

SetHref sets Href field to given value.


### GetKind

`func (o *ServiceMetadata) GetKind() string`

GetKind returns the Kind field if non-nil, zero value otherwise.

### GetKindOk

`func (o *ServiceMetadata) GetKindOk() (*string, bool)`

GetKindOk returns a tuple with the Kind field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetKind

`func (o *ServiceMetadata) SetKind(v string)`

SetKind sets Kind field to given value.


### GetVersion

`func (o *ServiceMetadata) GetVersion() string`

GetVersion returns the Version field if non-nil, zero value otherwise.

### GetVersionOk

`func (o *ServiceMetadata) GetVersionOk() (*string, bool)`

GetVersionOk returns a tuple with the Version field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetVersion

`func (o *ServiceMetadata) SetVersion(v string)`

SetVersion sets Version field to given value.


### GetBuildTime

`func (o *ServiceMetadata) GetBuildTime() string`

GetBuildTime returns the BuildTime field if non-nil, zero value otherwise.

### GetBuildTimeOk

`func (o *ServiceMetadata) GetBuildTimeOk() (*string, bool)`

GetBuildTimeOk returns a tuple with the BuildTime field if it's non-nil, zero value otherwise
and a boolean to check if the value has been set.

### SetBuildTime

`func (o *ServiceMetadata) SetBuildTime(v string)`

SetBuildTime sets BuildTime field to given value.



[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


