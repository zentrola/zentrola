package bootstrap

import (
	"context"
	"errors"
	"time"

	"github.com/zentrola/zentrola/internal/domain/shared"
)

// DeepSeekStore 只增加独立目录，不覆盖原有 Claude 映射、权限和资源。
type DeepSeekStore interface {
	InstallDeepSeek(context.Context, Seed) error
}

// SetupDeepSeek 仅供隔离数据库测试构建目录夹具；生产命令不调用此函数。
func SetupDeepSeek(ctx context.Context, store DeepSeekStore, generator shared.IDGenerator) error {
	ids := make([]int64, 7)
	for i := range ids {
		id, err := generator.NextID()
		if err != nil {
			return errors.New("cannot generate DeepSeek catalog IDs")
		}
		ids[i] = id
	}
	return store.InstallDeepSeek(ctx, Seed{
		ProviderID: ids[0], CreatedAt: time.Now().UTC(),
		Models: []Model{
			{ID: ids[1], ProviderModelID: ids[2], OpenAIProviderModelID: ids[5], Code: "deepseek-v4-flash", Name: "DeepSeek V4 Flash", UpstreamCode: "deepseek-v4-flash"},
			{ID: ids[3], ProviderModelID: ids[4], OpenAIProviderModelID: ids[6], Code: "deepseek-v4-pro", Name: "DeepSeek V4 Pro", UpstreamCode: "deepseek-v4-pro"},
		},
	})
}
