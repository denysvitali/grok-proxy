package cmd

import (
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/denysvitali/grok-proxy/internal/auth"
	"github.com/denysvitali/grok-proxy/internal/config"
	"github.com/denysvitali/grok-proxy/internal/grok"
	"github.com/sirupsen/logrus"
)

type runtime struct {
	config config.Config
	log    *logrus.Logger
	tokens *auth.Manager
	grok   *grok.Client
}

func newRuntime() (*runtime, error) {
	_, cfg, err := config.New(options.configFile)
	if err != nil {
		return nil, err
	}
	if options.authFile != "" {
		cfg.AuthFile = options.authFile
	}
	if options.baseURL != "" {
		cfg.BaseURL = options.baseURL
	}
	if options.logLevel != "" {
		cfg.LogLevel = options.logLevel
	}
	if options.logFormat != "" {
		cfg.LogFormat = options.logFormat
	}
	if options.noColor {
		cfg.NoColor = true
	}

	logger, err := newLogger(cfg)
	if err != nil {
		return nil, err
	}
	tokens := &auth.Manager{
		Store:      &auth.Store{Path: cfg.AuthFile},
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		LegacyPath: config.LegacyAuthFile(),
		GrokPath:   config.GrokAuthFile(),
	}
	client := grok.New(cfg.BaseURL, tokens)
	client.HTTP = grok.DefaultHTTPClient()
	return &runtime{config: cfg, log: logger, tokens: tokens, grok: client}, nil
}

func newLogger(cfg config.Config) (*logrus.Logger, error) {
	logger := logrus.New()
	level, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		return nil, err
	}
	logger.SetLevel(level)
	logger.SetOutput(os.Stderr)
	switch cfg.LogFormat {
	case "text":
		logger.SetFormatter(&logrus.TextFormatter{DisableColors: cfg.NoColor, FullTimestamp: true})
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{})
	default:
		return nil, errors.New("log format must be text or json")
	}
	return logger, nil
}
