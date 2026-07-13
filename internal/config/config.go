package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const (
	Issuer        = "https://auth.x.ai"
	ClientID      = "b1a00492-073a-47ea-816f-4c329264a828"
	Scopes        = "openid profile email offline_access grok-cli:access api:access"
	APIBase       = "https://cli-chat-proxy.grok.com/v1"
	ClientVersion = "0.2.99"
)

type Config struct {
	AuthFile  string       `mapstructure:"auth_file"`
	BaseURL   string       `mapstructure:"base_url"`
	LogLevel  string       `mapstructure:"log_level"`
	LogFormat string       `mapstructure:"log_format"`
	NoColor   bool         `mapstructure:"no_color"`
	Server    ServerConfig `mapstructure:"server"`
	Proxy     ProxyConfig  `mapstructure:"proxy"`
}

type ServerConfig struct {
	Listen        string `mapstructure:"listen"`
	APIKey        string `mapstructure:"api_key"`
	AllowInsecure bool   `mapstructure:"allow_insecure"`
	MaxBodyBytes  int64  `mapstructure:"max_body_bytes"`
}

type ProxyConfig struct {
	DefaultModel string            `mapstructure:"default_model"`
	ModelMap     map[string]string `mapstructure:"model_map"`
}

func DefaultAuthFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "grok-proxy", "auth.json")
}

func LegacyAuthFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "grok-subscription-client", "auth.json")
}

func GrokAuthFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".grok", "auth.json")
}

func New(configFile string) (*viper.Viper, Config, error) {
	v := viper.New()
	v.SetDefault("auth_file", DefaultAuthFile())
	v.SetDefault("base_url", APIBase)
	v.SetDefault("log_level", "info")
	v.SetDefault("log_format", "text")
	v.SetDefault("server.listen", "127.0.0.1:8080")
	v.SetDefault("server.max_body_bytes", int64(16<<20))
	v.SetDefault("proxy.default_model", "grok-4.5")
	v.SetDefault("proxy.model_map", map[string]string{})
	v.SetEnvPrefix("GROK_PROXY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	_ = v.BindEnv("server.api_key", "GROK_PROXY_API_KEY")

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		home, _ := os.UserHomeDir()
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(filepath.Join(home, ".config", "grok-proxy"))
	}
	if err := v.ReadInConfig(); err != nil {
		if configFile != "" {
			return nil, Config{}, fmt.Errorf("read config: %w", err)
		}
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, Config{}, fmt.Errorf("read config: %w", err)
		}
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, Config{}, fmt.Errorf("decode config: %w", err)
	}
	return v, cfg, nil
}

func (c Config) ResolveModel(requested string) string {
	if mapped := c.Proxy.ModelMap[requested]; mapped != "" {
		return mapped
	}
	if strings.HasPrefix(requested, "grok-") {
		return requested
	}
	return c.Proxy.DefaultModel
}
