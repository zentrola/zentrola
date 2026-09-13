package security

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
)

type MasterKey struct {
	value []byte
}

// LoadMasterKey 只接受配置文件中的 MASTER_KEY；错误不包含密钥内容。
func LoadMasterKey(encoded string) (*MasterKey, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, errors.New("MASTER_KEY is required")
	}
	return decodeMaster(encoded)
}

func decodeMaster(encoded string) (*MasterKey, error) {
	value, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(value) != 32 {
		clear(value)
		return nil, errors.New("Master Key must contain exactly 32 Base64-encoded bytes")
	}
	return &MasterKey{value: value}, nil
}
func (k *MasterKey) Check(context.Context) error {
	if k == nil || len(k.value) != 32 {
		return errors.New("Master Key unavailable")
	}
	return nil
}
