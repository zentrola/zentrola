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
		return "", errors.New("cannot determine the application directory")
	}
	return filepath.Join(filepath.Dir(executable), "run"), nil
}

func requireConfigFile(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("config file %q does not exist or is not readable; use --config <path> to select another file", path)
	}
	return nil
}

func selectConfig(command commandOptions) (configSelection, error) {
	selection := configSelection{path: command.configPath, pinned: command.configProvided}
	abs, err := filepath.Abs(selection.path)
	if err != nil {
		return selection, errors.New("cannot resolve the config file path")
	}
	selection.path = abs
	if selection.pinned {
		if err := requireConfigFile(abs); err != nil {
			return selection, err
		}
	}
	return selection, nil
}
