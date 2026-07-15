package proxy

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/denysvitali/grok-proxy/internal/auth"
	"github.com/denysvitali/grok-proxy/internal/config"
	"github.com/denysvitali/grok-proxy/internal/grok"
	"github.com/sirupsen/logrus"
)

type Server struct {
	config          config.Config
	grok            *grok.Client
	tokens          *auth.Manager
	dashboardClient *grok.DashboardClient
	log             *logrus.Logger
}

func New(cfg config.Config, client *grok.Client, tokens *auth.Manager, logger *logrus.Logger) *Server {
	var httpClient *http.Client
	if tokens != nil {
		httpClient = tokens.HTTPClient
	}
	return &Server{config: cfg, grok: client, tokens: tokens, dashboardClient: grok.NewDashboardClient(cfg.BaseURL, httpClient), log: logger}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.authenticate(s.dashboard))
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /readyz", s.ready)
	mux.Handle("GET /metrics", metricsHandler())
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("GET /v1/models", s.authenticate(s.models))
	mux.HandleFunc("POST /v1/responses", s.authenticate(s.responses))
	mux.HandleFunc("POST /v1/messages", s.authenticate(s.messages))
	mux.HandleFunc("POST /v1/messages/count_tokens", s.authenticate(s.countTokens))
	return s.recoverPanics(s.logRequests(s.withRequestID(mux)))
}

func (s *Server) ValidateListenAddress() error {
	host, _, err := net.SplitHostPort(s.config.Server.Listen)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	ip := net.ParseIP(host)
	isLoopback := host == "localhost" || ip != nil && ip.IsLoopback()
	if !isLoopback && s.config.Server.APIKey == "" && !s.config.Server.AllowInsecure {
		return errors.New("refusing a non-loopback listener without GROK_PROXY_API_KEY; use --allow-insecure to override")
	}
	return nil
}

type healthResponse struct {
	Status string `json:"status"`
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func (s *Server) ready(w http.ResponseWriter, _ *http.Request) {
	if s.tokens != nil && s.tokens.Store != nil && s.tokens.Store.Path != "" {
		if _, err := os.Stat(s.tokens.Store.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "auth_store_unavailable"})
			return
		}
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ready"})
}
