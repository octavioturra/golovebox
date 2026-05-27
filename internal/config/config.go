package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	LLMProvider   string `toml:"llm_provider"`
	LLMBaseURL    string `toml:"llm_base_url"`
	LLMModel      string `toml:"llm_model"`
	APIKey        string `toml:"api_key"`
	GitHubToken   string `toml:"github_token"`
	TelegramToken string `toml:"telegram_token"`
	QEMUPath      string `toml:"qemu_path"`
	VMPath        string `toml:"vm_path"`
	SSHPort       int    `toml:"ssh_port"`
	QMPPort       int    `toml:"qmp_port"`
}

var providerBaseURLs = map[string]string{
	"anthropic": "https://api.anthropic.com",
	"openai":    "https://api.openai.com",
	"ollama":    "http://localhost:11434",
	"gemini":    "https://generativelanguage.googleapis.com",
}

// DefaultBaseURL returns the default API base URL for a given LLM provider name.
func DefaultBaseURL(provider string) string {
	return providerBaseURLs[provider]
}

func BaseDir() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(execPath), ".golovebox"), nil
}

func (c *Config) ConfigFile() (string, error) {
	bd, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(bd, "config.toml"), nil
}

func (c *Config) QEMUDir() (string, error) {
	bd, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(bd, "qemu"), nil
}

func (c *Config) VMDir() (string, error) {
	bd, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(bd, "vm"), nil
}

func (c *Config) MemoryDir() (string, error) {
	bd, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(bd, "memory"), nil
}

func (c *Config) LogsDir() (string, error) {
	bd, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(bd, "logs"), nil
}

func Load() (*Config, error) {
	cfg := &Config{
		SSHPort: 2222,
		QMPPort: 4444,
	}
	bd, err := BaseDir()
	if err != nil {
		return nil, err
	}
	cf := filepath.Join(bd, "config.toml")
	if _, statErr := os.Stat(cf); os.IsNotExist(statErr) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(cf, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Save(cfg *Config) error {
	cf, err := cfg.ConfigFile()
	if err != nil {
		return err
	}
	f, err := os.Create(cf)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
