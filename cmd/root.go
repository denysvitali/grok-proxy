package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var version = "dev"

type globalOptions struct {
	configFile string
	authFile   string
	baseURL    string
	logLevel   string
	logFormat  string
	noColor    bool
}

var options globalOptions

func Execute() int {
	root := newRootCommand()
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, errorStyle().Render("error: ")+err.Error())
		return 1
	}
	return 0
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "grok-proxy",
		Short:         "Use a Grok subscription from CLIs and compatible APIs",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	flags := root.PersistentFlags()
	flags.StringVar(&options.configFile, "config", "", "configuration file")
	flags.StringVar(&options.authFile, "auth-file", "", "credential file")
	flags.StringVar(&options.baseURL, "base-url", "", "Grok subscription API base URL")
	flags.StringVar(&options.logLevel, "log-level", "", "log level")
	flags.StringVar(&options.logFormat, "log-format", "", "text or json logging")
	flags.BoolVar(&options.noColor, "no-color", false, "disable colored output")

	root.AddCommand(
		newLoginCommand(),
		newImportCommand(),
		newLogoutCommand(),
		newModelsCommand(),
		newChatCommand(),
		newServeCommand(),
		newVersionCommand(),
	)
	root.InitDefaultCompletionCmd()
	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(command *cobra.Command, _ []string) {
			fmt.Fprintln(command.OutOrStdout(), "grok-proxy "+version)
		},
	}
}

func infoStyle() lipgloss.Style {
	if options.noColor || os.Getenv("NO_COLOR") != "" {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
}

func errorStyle() lipgloss.Style {
	if options.noColor || os.Getenv("NO_COLOR") != "" {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
}
