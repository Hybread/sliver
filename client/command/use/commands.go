package use

import (
	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/bishopfox/sliver/client/command/pulses"
	"github.com/bishopfox/sliver/client/command/flags"
	"github.com/bishopfox/sliver/client/command/help"
	"github.com/bishopfox/sliver/client/console"
	consts "github.com/bishopfox/sliver/client/constants"
)

// Commands returns the “ command and its subcommands.
func Commands(con *console.SliverClient) []*cobra.Command {
	useCmd := &cobra.Command{
		Use:   consts.UseStr,
		Short: "Switch the active session or pulse",
		Long:  help.GetHelpFor([]string{consts.UseStr}),
		Run: func(cmd *cobra.Command, args []string) {
			UseCmd(cmd, con, args)
		},
		GroupID: consts.SliverHelpGroup,
	}
	flags.Bind("use", true, useCmd, func(f *pflag.FlagSet) {
		f.Int64P("timeout", "t", flags.DefaultTimeout, "grpc timeout in seconds")
	})
	carapace.Gen(useCmd).PositionalCompletion(PulseAndSessionIDCompleter(con))
	registerUseIDCompletion(useCmd, con, true, true)

	useSessionCmd := &cobra.Command{
		Use:   consts.SessionsStr,
		Short: "Switch the active session",
		Long:  help.GetHelpFor([]string{consts.UseStr, consts.SessionsStr}),
		Run: func(cmd *cobra.Command, args []string) {
			UseSessionCmd(cmd, con, args)
		},
	}
	carapace.Gen(useSessionCmd).PositionalCompletion(SessionIDCompleter(con))
	registerUseIDCompletion(useSessionCmd, con, true, false)
	useCmd.AddCommand(useSessionCmd)

	usePulseCmd := &cobra.Command{
		Use:   consts.PulsesStr,
		Short: "Switch the active pulse",
		Long:  help.GetHelpFor([]string{consts.UseStr, consts.PulsesStr}),
		Run: func(cmd *cobra.Command, args []string) {
			UsePulseCmd(cmd, con, args)
		},
	}
	carapace.Gen(usePulseCmd).PositionalCompletion(pulses.PulseIDCompleter(con))
	registerUseIDCompletion(usePulseCmd, con, false, true)
	useCmd.AddCommand(usePulseCmd)

	return []*cobra.Command{useCmd}
}
