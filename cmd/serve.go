package cmd

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/denysvitali/grok-proxy/internal/proxy"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newServeCommand() *cobra.Command {
	var listen string
	var defaultModel string
	var allowInsecure bool
	command := &cobra.Command{
		Use:   "serve",
		Short: "Serve Anthropic and OpenAI-compatible APIs",
		RunE: func(command *cobra.Command, _ []string) error {
			runtime, err := newRuntime()
			if err != nil {
				return err
			}
			if command.Flags().Changed("listen") {
				runtime.config.Server.Listen = listen
			}
			if command.Flags().Changed("model") {
				runtime.config.Proxy.DefaultModel = defaultModel
			}
			if allowInsecure {
				runtime.config.Server.AllowInsecure = true
			}

			handler := proxy.New(runtime.config, runtime.grok, runtime.tokens, runtime.log)
			if err := handler.ValidateListenAddress(); err != nil {
				return err
			}
			server := &http.Server{
				Addr:              runtime.config.Server.Listen,
				Handler:           handler.Handler(),
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       2 * time.Minute,
			}

			ctx, stop := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			go shutdownServer(ctx, server)

			runtime.log.WithFields(logrus.Fields{
				"listen":      runtime.config.Server.Listen,
				"model":       runtime.config.Proxy.DefaultModel,
				"client_auth": runtime.config.Server.APIKey != "",
			}).Info("Grok proxy listening")
			err = server.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		},
	}
	command.Flags().StringVar(&listen, "listen", "127.0.0.1:8080", "listen address")
	command.Flags().StringVarP(&defaultModel, "model", "m", "grok-4.5", "default Grok model")
	command.Flags().BoolVar(&allowInsecure, "allow-insecure", false, "allow unauthenticated non-loopback listening")
	return command
}

func shutdownServer(ctx context.Context, server *http.Server) {
	<-ctx.Done()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownContext)
}
