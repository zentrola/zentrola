// Package bootstrap 编排首次数据库初始化，不承担管理员认证或 Credential 初始化。
package bootstrap

import (
	"context"
	"errors"
	"time"

	"github.com/zentrola/zentrola/internal/domain/shared"
)

type Seed struct {
	OrganizationID int64
	Providers      []Provider
	CreatedAt      time.Time
}

type Provider struct {
	ID        int64
	Code      string
	Name      string
	Endpoints []Endpoint
}

type Endpoint struct {
	ProtocolType string
	BaseURL      string
}

var providerTemplates = [...]Provider{
	{
		Code: "openai-official",
		Name: "OpenAI Official",
		Endpoints: []Endpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://api.openai.com/v1"},
		},
	},
	{
		Code: "anthropic-official",
		Name: "Anthropic Official",
		Endpoints: []Endpoint{
			{ProtocolType: "ANTHROPIC", BaseURL: "https://api.anthropic.com"},
		},
	},
	{
		Code: "google-gemini-official",
		Name: "Google Gemini Official",
		Endpoints: []Endpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai"},
		},
	},
	{
		Code: "deepseek-official",
		Name: "DeepSeek Official",
		Endpoints: []Endpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://api.deepseek.com"},
			{ProtocolType: "ANTHROPIC", BaseURL: "https://api.deepseek.com/anthropic"},
		},
	},
	{
		Code: "zhipu-official",
		Name: "Zhipu AI Official",
		Endpoints: []Endpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://open.bigmodel.cn/api/paas/v4"},
			{ProtocolType: "ANTHROPIC", BaseURL: "https://open.bigmodel.cn/api/anthropic"},
		},
	},
	{
		Code: "kimi-official",
		Name: "Kimi Official",
		Endpoints: []Endpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://api.moonshot.cn/v1"},
		},
	},
	{
		Code: "qwen-official",
		Name: "Qwen Official",
		Endpoints: []Endpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
			{ProtocolType: "ANTHROPIC", BaseURL: "https://dashscope.aliyuncs.com/apps/anthropic"},
		},
	},
}

// OfficialProviderTemplates 返回独立副本，供首次安装和管理员补齐预置厂商共用。
func OfficialProviderTemplates() []Provider {
	result := make([]Provider, len(providerTemplates))
	for index, template := range providerTemplates {
		result[index] = template
		result[index].Endpoints = append([]Endpoint(nil), template.Endpoints...)
	}
	return result
}

type Store interface {
	// InitializeOnce 必须在同一数据库事务内检查并写入，不能更新已有业务数据。
	InitializeOnce(context.Context, Seed) error
	Initialized(context.Context) (bool, error)
}

type Service struct {
	store Store
	ids   shared.IDGenerator
}

func New(store Store, ids shared.IDGenerator) *Service {
	return &Service{store: store, ids: ids}
}

func (s *Service) Initialize(ctx context.Context) error {
	organizationID, err := s.ids.NextID()
	if err != nil {
		return errors.New("cannot generate bootstrap organization ID")
	}
	providers := OfficialProviderTemplates()
	for index := range providers {
		id, err := s.ids.NextID()
		if err != nil {
			return errors.New("cannot generate bootstrap provider IDs")
		}
		providers[index].ID = id
	}
	return s.store.InitializeOnce(ctx, Seed{
		OrganizationID: organizationID,
		Providers:      providers,
		CreatedAt:      time.Now().UTC(),
	})
}

func (s *Service) Check(ctx context.Context) error {
	initialized, err := s.store.Initialized(ctx)
	if err != nil {
		return err
	}
	if !initialized {
		return errors.New("bootstrap not initialized")
	}
	return nil
}
