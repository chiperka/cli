package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"chiperka-cli/internal/telemetry"
)

var telemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Manage anonymous telemetry",
	Long: `Manage anonymous usage telemetry for Chiperka.

Chiperka collects anonymous usage data to help improve the tool.
No personal information is collected. You can disable telemetry
at any time using 'chiperka telemetry disable'.

Environment variables:
  DO_NOT_TRACK=1  Disable telemetry (standard)`,
}

var telemetryEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable anonymous telemetry",
	Run: func(cmd *cobra.Command, args []string) {
		telemetry.SaveConfig(&telemetry.TelemetryConfig{
			Enabled:     true,
			NoticeShown: true,
		})
		fmt.Println("Telemetry enabled.")
	},
}

var telemetryDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable anonymous telemetry",
	Run: func(cmd *cobra.Command, args []string) {
		telemetry.SaveConfig(&telemetry.TelemetryConfig{
			Enabled:     false,
			NoticeShown: true,
		})
		fmt.Println("Telemetry disabled.")
	},
}

var telemetryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show telemetry status",
	Run: func(cmd *cobra.Command, args []string) {
		disabled := telemetry.IsDisabled()

		if disabled {
			fmt.Println("Status: disabled")
		} else {
			fmt.Println("Status: enabled")
		}

		// Show env var overrides
		if os.Getenv("DO_NOT_TRACK") == "1" {
			fmt.Println("  (disabled via DO_NOT_TRACK=1)")
		}
		mcfg := telemetry.LoadMachineConfig()
		if mcfg != nil {
			fmt.Printf("  config: enabled=%v, updated=%s\n", mcfg.Telemetry.Enabled, mcfg.UpdatedAt.Format("2006-01-02 15:04:05"))
		} else {
			fmt.Println("  config: not set (first run will show notice)")
		}
	},
}

func init() {
	rootCmd.AddCommand(telemetryCmd)
	telemetryCmd.AddCommand(telemetryEnableCmd)
	telemetryCmd.AddCommand(telemetryDisableCmd)
	telemetryCmd.AddCommand(telemetryStatusCmd)
}
