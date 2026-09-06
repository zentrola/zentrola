package security

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type MasterKey struct {
	value   []byte
	Created bool
}

// LoadMasterKey：显式值 > 外部 Secret 文件 > 本地持久化文件 > 首次生成。
// 错误不包含密钥内容；损坏的现有文件不能被悄悄替换。
func LoadMasterKey(encoded, externalPath, localPath string) (*MasterKey, error) {
	if encoded != "" {
		return decodeMaster(encoded, false)
	}
	if externalPath != "" {
		data, err := os.ReadFile(externalPath)
		if err != nil {
			return nil, errors.New("cannot read external Master Key")
		}
		defer clear(data)
		return decodeMaster(string(data), false)
	}
	if localPath == "" {
		return nil, errors.New("MASTER_KEY_PATH is required")
	}
	if _, err := os.Lstat(localPath); err == nil {
		return readLocalMaster(localPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("cannot inspect Master Key file")
	}
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.New("cannot create Master Key directory")
	}
	file, err := os.CreateTemp(dir, ".master-key-*")
	if err != nil {
		return nil, errors.New("cannot create temporary Master Key file")
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := protectSecret(file.Name()); err != nil {
		return nil, errors.New("cannot protect Master Key file")
	}
	var value [32]byte
	_, _ = rand.Read(value[:])
	defer clear(value[:])
	encoded = base64.StdEncoding.EncodeToString(value[:]) + "\n"
	if _, err := file.WriteString(encoded); err != nil {
		return nil, errors.New("cannot write Master Key")
	}
	if err := file.Sync(); err != nil {
		return nil, errors.New("cannot persist Master Key")
	}
	if err := file.Close(); err != nil {
		return nil, errors.New("cannot close Master Key file")
	}
	// 原子发布且绝不覆盖已有文件；并发首次启动读取获胜者写入的 Key。
	if err := os.Link(file.Name(), localPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return readLocalMaster(localPath)
		}
		return nil, errors.New("cannot atomically publish Master Key file")
	}
	if err := syncSecretDirectory(dir); err != nil {
		return nil, errors.New("cannot persist Master Key directory")
	}
	return decodeMaster(encoded, true)
}

func readLocalMaster(path string) (*MasterKey, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("Master Key must be a regular file")
	}
	if err := protectSecret(path); err != nil {
		return nil, errors.New("cannot protect Master Key file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("cannot read Master Key file")
	}
	defer clear(data)
	return decodeMaster(string(data), false)
}
func decodeMaster(encoded string, created bool) (*MasterKey, error) {
	value, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(value) != 32 {
		clear(value)
		return nil, errors.New("Master Key must contain exactly 32 Base64-encoded bytes")
	}
	return &MasterKey{value: value, Created: created}, nil
}
func (k *MasterKey) Check(context.Context) error {
	if k == nil || len(k.value) != 32 {
		return errors.New("Master Key unavailable")
	}
	return nil
}
