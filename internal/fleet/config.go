package fleet

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

var (
	ErrConfigNotFound  = errors.New("Config not found")
	ErrConfigMalformed = errors.New("Config is malformed")
)

type NodeGroup struct {
	Model     string `yaml:"model"`
	GPUCount  int    `yaml:"gpuCount"`
	NodeCount int    `yaml:"nodeCount"`
}

type Config struct {
	NodeGroups []NodeGroup `yaml:"nodeGroups"`
}

func LoadConfig(path string) (*Config, error) {
	cfgYaml, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file, %s. err: %w", path, ErrConfigNotFound)
	}

	var config Config
	if err := yaml.Unmarshal(cfgYaml, &config); err != nil {
		return nil, fmt.Errorf("failed to load config file %s. err %w", path, ErrConfigMalformed)
	}

	return &config, nil
}
