package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/zentrola/zentrola/internal/infrastructure/config"
)

type configBinding struct {
	File string `json:"file"`
}

type configSelection struct {
	path, source string
	pinned       bool
}

func bindingFile() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", errors.New("无法确定程序目录")
	}
	return filepath.Join(filepath.Dir(executable), "config.json"), nil
}

func requireConfigFile(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("配置文件 %q 不存在或不可读取；请用 config --file <路径> 重新绑定", path)
	}
	return nil
}

func selectConfig(command commandOptions, bindingPath string) (configSelection, error) {
	selection := configSelection{path: command.configPath, source: "当前工作目录", pinned: command.configProvided}
	if command.configProvided {
		selection.source = "单次 --config"
	} else {
		data, err := os.ReadFile(bindingPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return selection, errors.New("无法读取配置绑定；可用 --config 临时指定或 config --file 重新绑定")
		}
		if err == nil {
			var binding configBinding
			if json.Unmarshal(data, &binding) != nil || !filepath.IsAbs(binding.File) {
				return selection, errors.New("配置绑定格式无效；请用 config --file <路径> 重新绑定")
			}
			selection = configSelection{path: binding.File, source: "已保存绑定", pinned: true}
		}
	}
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

func saveConfigBinding(bindingPath, file string) (configSelection, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return configSelection{}, errors.New("无法解析配置文件路径")
	}
	selection := configSelection{path: abs, source: "已保存绑定", pinned: true}
	if err := requireConfigFile(abs); err != nil {
		return selection, err
	}
	// 仅验证分层文件语法和环境选择，具体参数由对应命令验证。
	if _, err := config.Environment(abs); err != nil {
		return selection, err
	}
	data, err := json.MarshalIndent(configBinding{File: abs}, "", "  ")
	if err != nil {
		return selection, errors.New("无法编码配置绑定")
	}
	tmp, err := os.CreateTemp(filepath.Dir(bindingPath), ".zentrola-config-*")
	if err != nil {
		return selection, errors.New("无法保存配置绑定；请确认程序目录可写")
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return selection, errors.New("写入配置绑定失败")
	}
	if err := tmp.Sync(); err != nil {
		return selection, errors.New("保存配置绑定失败")
	}
	if err := tmp.Close(); err != nil {
		return selection, errors.New("关闭配置绑定文件失败")
	}
	if err := os.Rename(tmp.Name(), bindingPath); err != nil {
		return selection, errors.New("替换配置绑定失败；原绑定保持不变")
	}
	return selection, nil
}

func configure(command commandOptions, bindingPath string, output io.Writer) error {
	var selection configSelection
	var err error
	if command.configFile != "" {
		selection, err = saveConfigBinding(bindingPath, command.configFile)
	} else {
		selection, err = selectConfig(command, bindingPath)
	}
	if err != nil {
		return err
	}
	environment, err := config.Environment(selection.path)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "配置文件：%s\n来源：%s\n环境：%s\n绑定记录：%s\n", selection.path, selection.source, environment, bindingPath)
	return err
}
