package cmd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/denysvitali/grok-proxy/internal/auth"
	"github.com/denysvitali/grok-proxy/internal/config"
	"github.com/spf13/cobra"
)

func newLoginCommand() *cobra.Command {
	var device bool
	var issuer string
	var clientID string
	var scopes string
	command := &cobra.Command{
		Use:   "login",
		Short: "Sign in through xAI OAuth",
		RunE: func(command *cobra.Command, _ []string) error {
			runtime, err := newRuntime()
			if err != nil {
				return err
			}
			announce := func(message string) {
				fmt.Fprintln(command.ErrOrStderr(), infoStyle().Render(message))
			}
			client := &http.Client{Timeout: 30 * time.Second}
			if device {
				err = auth.LoginDevice(command.Context(), client, runtime.tokens.Store, issuer, clientID, scopes, announce)
			} else {
				err = auth.LoginBrowser(command.Context(), client, runtime.tokens.Store, issuer, clientID, scopes, announce)
			}
			if err == nil {
				announce("Login successful.")
			}
			return err
		},
	}
	command.Flags().BoolVar(&device, "device", false, "use device authorization")
	command.Flags().StringVar(&issuer, "issuer", config.Issuer, "OIDC issuer")
	command.Flags().StringVar(&clientID, "client-id", config.ClientID, "OAuth client ID")
	command.Flags().StringVar(&scopes, "scopes", config.Scopes, "OAuth scopes")
	return command
}

func newImportCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "import-grok",
		Short: "Import credentials from ~/.grok/auth.json",
		RunE: func(command *cobra.Command, _ []string) error {
			runtime, err := newRuntime()
			if err != nil {
				return err
			}
			token, err := auth.ImportGrok(config.GrokAuthFile())
			if err != nil {
				return err
			}
			if err := runtime.tokens.Store.Save(token); err != nil {
				return err
			}
			fmt.Fprintln(command.ErrOrStderr(), infoStyle().Render("Imported credentials into "+runtime.config.AuthFile))
			return nil
		},
	}
}

func newLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove local credentials",
		RunE: func(command *cobra.Command, _ []string) error {
			runtime, err := newRuntime()
			if err != nil {
				return err
			}
			if err := runtime.tokens.Store.Clear(); err != nil {
				return err
			}
			fmt.Fprintln(command.ErrOrStderr(), infoStyle().Render("Local credentials removed."))
			return nil
		},
	}
}
