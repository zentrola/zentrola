// Package idgen 使用 Sonyflake 生成正数 int64 ID，适配 PostgreSQL BIGINT。
package idgen

import (
	"errors"
	"time"

	"github.com/sony/sonyflake/v2"
)

func New(nodeID int) (*sonyflake.Sonyflake, error) {
	if nodeID < 1 || nodeID > 65535 {
		return nil, errors.New("ID_NODE must be between 1 and 65535")
	}
	return sonyflake.New(sonyflake.Settings{
		StartTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		MachineID: func() (int, error) { return nodeID, nil },
	})
}
