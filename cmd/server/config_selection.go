package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type configSelection struct {
	path   string
	pinned bool
}

func executableRuntimeDir() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", errors.New("无法确定程序目录")
	}
	return filepath.Join(filepath.Dir(executable), "run"), nil
}

func requireConfigFile(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("配置文件 %q 不存在或不可读取；请检查路径或用 --config <路径> 重新指定", path)
	}
	return nil
}

func selectConfig(command commandOptions) (configSelection, error) {
	selection := configSelection{path: command.configPath, pinned: command.configProvided}
	abs, err := filepath.Abs(selection.path)
	if err != nil {
		return selection, errors.New("无法解析配置文件路径")
	}
	selection.path = abs
	if selection.pinned {
		if err := requireConfigFile(abs); err != nil {
			return selection, err
		}
	}
	return selection, nil
}
