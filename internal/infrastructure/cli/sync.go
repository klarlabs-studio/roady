package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var (
	syncPluginName string
	syncNoPush     bool
)

var syncCmd = &cobra.Command{
	Use:   "sync [plugin-path]",
	Short: "Sync the plan with an external system via a plugin",
	Long: `Sync the plan with an external system via a plugin.

You can use this command in two ways:

1. Using a named plugin configuration (recommended):
   roady sync --name my-jira

2. Using a plugin binary path directly (uses environment variables):
   roady sync ./roady-plugin-jira

Configure plugins in .roady/plugins.yaml:
  plugins:
    my-jira:
      binary: ./roady-plugin-jira
      config:
        domain: https://company.atlassian.net
        project_key: PROJ
        email: user@example.com
        api_token: your-token`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		services.Sync.SetPushEnabled(!syncNoPush)

		var results []string

		if syncPluginName != "" {
			// Use named plugin configuration
			results, err = services.Sync.SyncWithNamedPlugin(syncPluginName)
			if err != nil {
				return err
			}
		} else if len(args) == 1 {
			// Use plugin binary path (backward compatible)
			pluginPath := args[0]
			results, err = services.Sync.SyncWithPlugin(pluginPath)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("either --name flag or plugin-path argument is required")
		}

		if len(results) == 0 {
			fmt.Println("No updates from plugin.")
			return nil
		}

		identifier := syncPluginName
		if identifier == "" && len(args) > 0 {
			identifier = args[0]
		}
		fmt.Printf("Sync results for %s:\n", identifier)
		for _, res := range results {
			fmt.Printf("- %s\n", res)
		}

		return nil
	},
}

var syncListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured plugins",
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		names, err := services.Sync.ListPluginConfigs()
		if err != nil {
			return err
		}

		if len(names) == 0 {
			fmt.Println("No plugins configured. Add plugins to .roady/plugins.yaml")
			return nil
		}

		fmt.Println("Configured plugins:")
		sort.Strings(names)
		for _, name := range names {
			cfg, err := services.Sync.GetPluginConfig(name)
			if err != nil {
				fmt.Printf("  %s (error loading config)\n", name)
				continue
			}
			fmt.Printf("  %s → %s\n", name, cfg.Binary)
		}

		return nil
	},
}

var syncShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show a plugin configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		cfg, err := services.Sync.GetPluginConfig(args[0])
		if err != nil {
			return err
		}

		fmt.Printf("Plugin: %s\n", args[0])
		fmt.Printf("Binary: %s\n", cfg.Binary)
		fmt.Println("Config:")
		for k, v := range cfg.Config {
			// Mask sensitive values
			displayValue := v
			if isSensitiveKey(k) && len(v) > 4 {
				displayValue = v[:4] + "****"
			}
			fmt.Printf("  %s: %s\n", k, displayValue)
		}

		return nil
	},
}

func isSensitiveKey(key string) bool {
	sensitive := []string{"token", "api_token", "api_key", "password", "secret"}
	for _, s := range sensitive {
		if key == s {
			return true
		}
	}
	return false
}

func init() {
	syncCmd.Flags().StringVarP(&syncPluginName, "name", "n", "", "Use named plugin configuration from plugins.yaml")
	syncCmd.Flags().BoolVar(&syncNoPush, "no-push", false, "Pull external status only; do not write Roady's status back to the tracker")
	syncCmd.AddCommand(syncListCmd)
	syncCmd.AddCommand(syncShowCmd)
	RootCmd.AddCommand(syncCmd)
}
