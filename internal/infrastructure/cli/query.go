package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query [question]",
	Short: "Ask a natural language question about the project",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}
		question := strings.Join(args, " ")
		req, err := services.Prompt.QueryProject(cmd.Context(), question)
		if err != nil {
			return MapError(err)
		}

		return printPromptRequest(req, promptJSON)
	},
}

func init() {
	addPromptJSONFlag(queryCmd)
	RootCmd.AddCommand(queryCmd)
}
