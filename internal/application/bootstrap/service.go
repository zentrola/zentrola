// Package bootstrap 编排首次数据库基础组织初始化，不承担服务商、管理员认证或 Credential 初始化。
package bootstrap

import (
	"context"
	"errors"
	"time"

	"github.com/zentrola/zentrola/internal/domain/shared"
)

type Seed struct {
	OrganizationID int64
	CreatedAt      time.Time
}

type Provider struct {
	Code      string
	NameZH    string
	NameEN    string
	Website   string
	Endpoints []Endpoint
}

func (p Provider) LocalizedName(locale string) string {
	if locale == "zh-CN" {
		return p.NameZH
	}
	return p.NameEN
}

type Endpoint struct {
	ProtocolType string
	BaseURL      string
}

var providerTemplates = [...]Provider{
	{
		Code:    "openai-official",
		NameZH:  "OpenAI",
		NameEN:  "OpenAI",
		Website: "https://openai.com",
		Endpoints: []Endpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://api.openai.com/v1"},
		},
	},
	{
		Code:    "anthropic-official",
		NameZH:  "Anthropic",
		NameEN:  "Anthropic",
		Website: "https://www.anthropic.com",
		Endpoints: []Endpoint{
			{ProtocolType: "ANTHROPIC", BaseURL: "https://api.anthropic.com"},
		},
	},
	{
		Code:    "google-gemini-official",
		NameZH:  "Google",
		NameEN:  "Google",
		Website: "https://ai.google.dev/gemini-api",
		Endpoints: []Endpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai"},
		},
	},
	{
		Code:    "deepseek-official",
		NameZH:  "深度求索",
		NameEN:  "DeepSeek",
		Website: "https://www.deepseek.com",
		Endpoints: []Endpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://api.deepseek.com"},
			{ProtocolType: "ANTHROPIC", BaseURL: "https://api.deepseek.com/anthropic"},
		},
	},
	{
		Code:    "zhipu-official",
		NameZH:  "智谱 AI",
		NameEN:  "Zhipu AI",
		Website: "https://www.zhipuai.cn",
		Endpoints: []Endpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://open.bigmodel.cn/api/paas/v4"},
			{ProtocolType: "ANTHROPIC", BaseURL: "https://open.bigmodel.cn/api/anthropic"},
		},
	},
	{
		Code:    "kimi-official",
		NameZH:  "月之暗面",
		NameEN:  "Moonshot AI",
		Website: "https://www.moonshot.cn",
		Endpoints: []Endpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://api.moonshot.cn/v1"},
		},
	},
	{
		Code:    "qwen-official",
		NameZH:  "阿里云",
		NameEN:  "Alibaba Cloud",
		Website: "https://qwen.ai",
		Endpoints: []Endpoint{
			{ProtocolType: "OPENAI", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
			{ProtocolType: "ANTHROPIC", BaseURL: "https://dashscope.aliyuncs.com/apps/anthropic"},
		},
	},
}

// OfficialProviderTemplates 返回独立副本，供管理员显式初始化或补齐预置厂商使用。
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
	organizationID, err := s.ids.NextID(ctx)
	if err != nil {
		return errors.New("cannot generate bootstrap organization ID")
	}
	return s.store.InitializeOnce(ctx, Seed{
		OrganizationID: organizationID,
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
