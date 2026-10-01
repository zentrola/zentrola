package http

import mgmt "github.com/zentrola/zentrola/internal/application/management"

// ModelRequest 是模型管理的 HTTP 契约；application.ModelInput 不携带协议、
// 校验框架或文档标签，避免 API 字段变化向应用层扩散。
type ModelRequest struct {
	Code                string   `json:"code" binding:"required"`
	Name                string   `json:"name" binding:"required"`
	PublisherProviderID *int64   `json:"publisherProviderId,string"`
	InputModalities     []string `json:"inputModalities" binding:"required" enums:"TEXT,IMAGE,AUDIO,VIDEO"`
	OutputModalities    []string `json:"outputModalities" binding:"required" enums:"TEXT,IMAGE,AUDIO,VIDEO"`
	Remark              string   `json:"remark"`
}

func (request ModelRequest) modelInput() mgmt.ModelInput {
	return mgmt.ModelInput{
		Code: request.Code, Name: request.Name, PublisherProviderID: request.PublisherProviderID,
		InputModalities: request.InputModalities, OutputModalities: request.OutputModalities,
		Remark: request.Remark,
	}
}

func NewModelRequest(input mgmt.ModelInput) ModelRequest {
	return ModelRequest{
		Code: input.Code, Name: input.Name, PublisherProviderID: input.PublisherProviderID,
		InputModalities: input.InputModalities, OutputModalities: input.OutputModalities,
		Remark: input.Remark,
	}
}

func (request *ModelRequest) Normalize() {
	input := request.modelInput()
	input.Normalize()
	*request = NewModelRequest(input)
}

func (request ModelRequest) Valid() bool { return request.modelInput().Valid() }

type ProviderEndpointRequest struct {
	ProtocolType string `json:"protocolType" binding:"required" enums:"OPENAI,ANTHROPIC"`
	BaseURL      string `json:"baseUrl" binding:"required"`
	NetworkScope string `json:"networkScope" enums:"PUBLIC,PRIVATE"`
}

type ProviderProxyHeaderRequest struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value"`
}

type ProviderMappingRequest struct {
	ModelID           int64  `json:"modelId,string" binding:"required"`
	UpstreamModelCode string `json:"upstreamModelCode"`
	Priority          int32  `json:"priority,omitempty"`
}

type ProviderRequest struct {
	Name                   string                       `json:"name" binding:"required"`
	Website                string                       `json:"website"`
	Endpoints              []ProviderEndpointRequest    `json:"endpoints" binding:"required"`
	ProxyEnabled           bool                         `json:"proxyEnabled"`
	ProxyURL               string                       `json:"proxyUrl"`
	UpdateProxyCredentials bool                         `json:"updateProxyCredentials"`
	ProxyHeaders           []ProviderProxyHeaderRequest `json:"proxyHeaders"`
	Mappings               []ProviderMappingRequest     `json:"mappings" binding:"required" allowempty:"true"`
}

func (request ProviderRequest) providerInput() mgmt.ProviderInput {
	input := mgmt.ProviderInput{
		Name: request.Name, Website: request.Website, ProxyEnabled: request.ProxyEnabled,
		ProxyURL: request.ProxyURL, UpdateProxyCredentials: request.UpdateProxyCredentials,
	}
	if request.Endpoints != nil {
		input.Endpoints = make([]mgmt.ProviderEndpoint, len(request.Endpoints))
	}
	if request.ProxyHeaders != nil {
		input.ProxyHeaders = make([]mgmt.ProviderProxyHeaderInput, len(request.ProxyHeaders))
	}
	if request.Mappings != nil {
		input.Mappings = make([]mgmt.ProviderMappingInput, len(request.Mappings))
	}
	for index, endpoint := range request.Endpoints {
		input.Endpoints[index] = mgmt.ProviderEndpoint{ProtocolType: endpoint.ProtocolType, BaseURL: endpoint.BaseURL, NetworkScope: endpoint.NetworkScope}
	}
	for index, header := range request.ProxyHeaders {
		input.ProxyHeaders[index] = mgmt.ProviderProxyHeaderInput{Key: header.Key, Value: header.Value}
	}
	for index, mapping := range request.Mappings {
		input.Mappings[index] = mgmt.ProviderMappingInput{ModelID: mapping.ModelID, UpstreamModelCode: mapping.UpstreamModelCode, Priority: mapping.Priority}
	}
	return input
}

func NewProviderRequest(input mgmt.ProviderInput) ProviderRequest {
	request := ProviderRequest{
		Name: input.Name, Website: input.Website, ProxyEnabled: input.ProxyEnabled,
		ProxyURL: input.ProxyURL, UpdateProxyCredentials: input.UpdateProxyCredentials,
	}
	if input.Endpoints != nil {
		request.Endpoints = make([]ProviderEndpointRequest, len(input.Endpoints))
	}
	if input.ProxyHeaders != nil {
		request.ProxyHeaders = make([]ProviderProxyHeaderRequest, len(input.ProxyHeaders))
	}
	if input.Mappings != nil {
		request.Mappings = make([]ProviderMappingRequest, len(input.Mappings))
	}
	for index, endpoint := range input.Endpoints {
		request.Endpoints[index] = ProviderEndpointRequest{ProtocolType: endpoint.ProtocolType, BaseURL: endpoint.BaseURL, NetworkScope: endpoint.NetworkScope}
	}
	for index, header := range input.ProxyHeaders {
		request.ProxyHeaders[index] = ProviderProxyHeaderRequest{Key: header.Key, Value: header.Value}
	}
	for index, mapping := range input.Mappings {
		request.Mappings[index] = ProviderMappingRequest{ModelID: mapping.ModelID, UpstreamModelCode: mapping.UpstreamModelCode, Priority: mapping.Priority}
	}
	return request
}

func (request *ProviderRequest) Normalize() {
	input := request.providerInput()
	input.Normalize()
	*request = NewProviderRequest(input)
}

func (request ProviderRequest) Valid() bool { return request.providerInput().Valid() }

type ProviderInitializeRequest struct {
	Locale        string   `json:"locale" binding:"required" enums:"zh-CN,en-US"`
	ProviderCodes []string `json:"providerCodes" binding:"required"`
}

func (request ProviderInitializeRequest) providerInitializeInput() mgmt.ProviderInitializeInput {
	return mgmt.ProviderInitializeInput{Locale: request.Locale, ProviderCodes: request.ProviderCodes}
}

func (request *ProviderInitializeRequest) Normalize() {
	input := request.providerInitializeInput()
	input.Normalize()
	request.Locale, request.ProviderCodes = input.Locale, input.ProviderCodes
}

func (request ProviderInitializeRequest) Valid() bool {
	return request.providerInitializeInput().Valid()
}
