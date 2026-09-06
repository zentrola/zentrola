// Package bootstrap 编排首次数据库初始化，不承担管理员认证或 Credential 初始化。
package bootstrap

import (
	"context"
	"errors"
	"time"

	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/domain/shared"
)

type Model struct {
	ID                    int64
	ProviderModelID       int64
	OpenAIProviderModelID int64
	Code                  string
	Name                  string
	UpstreamCode          string
}

type Seed struct {
	OrganizationID int64
	ProviderID     int64
	Models         []Model
	CreatedAt      time.Time
}

type Store interface {
	// InitializeOnce 必须在同一数据库事务内检查并写入，不能更新已有业务数据。
	InitializeOnce(context.Context, Seed) error
	Initialized(context.Context) (bool, error)
}

type Service struct {
	store  Store
	ids    shared.IDGenerator
	sonnet string
	opus   string
}

func New(store Store, ids shared.IDGenerator, sonnet, opus string) *Service {
	return &Service{store: store, ids: ids, sonnet: sonnet, opus: opus}
}

func (s *Service) Initialize(ctx context.Context) error {
	ids := make([]int64, 6)
	for i := range ids {
		id, err := s.ids.NextID()
		if err != nil {
			return errors.New("cannot generate bootstrap IDs")
		}
		ids[i] = id
	}
	return s.store.InitializeOnce(ctx, Seed{
		OrganizationID: ids[0], ProviderID: ids[1], CreatedAt: time.Now().UTC(),
		Models: []Model{
			{ID: ids[2], ProviderModelID: ids[3], Code: catalog.SonnetCode, Name: "Claude Sonnet", UpstreamCode: s.sonnet},
			{ID: ids[4], ProviderModelID: ids[5], Code: catalog.OpusCode, Name: "Claude Opus", UpstreamCode: s.opus},
		},
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
