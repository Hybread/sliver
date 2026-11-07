package pulses

import (
	"context"
	"fmt"
	"strings"

	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/bishopfox/sliver/client/command/flags"
	"github.com/bishopfox/sliver/client/command/help"
	"github.com/bishopfox/sliver/client/console"
	consts "github.com/bishopfox/sliver/client/constants"
	"github.com/bishopfox/sliver/protobuf/commonpb"
)

// Commands returns the “ command and its subcommands.
func Commands(con *console.SliverClient) []*cobra.Command {
	pulsesCmd := &cobra.Command{
		Use:     consts.PulsesStr,
		Short:   "Manage pulses",
		Long:    help.GetHelpFor([]string{consts.PulsesStr}),
		GroupID: consts.SliverHelpGroup,
		Run: func(cmd *cobra.Command, args []string) {
			PulsesCmd(cmd, con, args)
		},
	}
	flags.Bind("pulses", true, pulsesCmd, func(f *pflag.FlagSet) {
		f.Int64P("timeout", "t", flags.DefaultTimeout, "grpc timeout in seconds")
	})
	flags.Bind("pulses", false, pulsesCmd, func(f *pflag.FlagSet) {
		f.StringP("kill", "k", "", "kill the designated pulse")
		f.BoolP("kill-all", "K", false, "kill all pulses")
		f.StringP("interact", "i", "", "interact with a pulse")
		f.BoolP("force", "F", false, "force killing the pulse")

		f.StringP("filter", "f", "", "filter pulses by substring")
		f.StringP("filter-re", "e", "", "filter pulses by regular expression")
	})
	flags.BindFlagCompletions(pulsesCmd, func(comp *carapace.ActionMap) {
		(*comp)["kill"] = PulseIDCompleter(con)
		(*comp)["interact"] = PulseIDCompleter(con)
	})
	registerPulseIDFlagCompletion(pulsesCmd, "interact", con)
	pulsesRmCmd := &cobra.Command{
		Use:   consts.RmStr,
		Short: "Remove a pulse",
		Long:  help.GetHelpFor([]string{consts.PulsesStr, consts.RmStr}),
		Run: func(cmd *cobra.Command, args []string) {
			PulsesRmCmd(cmd, con, args)
		},
	}
	carapace.Gen(pulsesRmCmd).PositionalCompletion(PulseIDCompleter(con))
	pulsesCmd.AddCommand(pulsesRmCmd)

	pulsesWatchCmd := &cobra.Command{
		Use:   consts.WatchStr,
		Short: "Watch your pulses",
		Long:  help.GetHelpFor([]string{consts.PulsesStr, consts.WatchStr}),
		Run: func(cmd *cobra.Command, args []string) {
			PulsesWatchCmd(cmd, con, args)
		},
	}
	pulsesCmd.AddCommand(pulsesWatchCmd)

	pulsesPruneCmd := &cobra.Command{
		Use:   consts.PruneStr,
		Short: "Prune stale pulses automatically",
		Long:  help.GetHelpFor([]string{consts.PulsesStr, consts.PruneStr}),
		Run: func(cmd *cobra.Command, args []string) {
			PulsesPruneCmd(cmd, con, args)
		},
	}
	flags.Bind("pulses", false, pulsesPruneCmd, func(f *pflag.FlagSet) {
		f.StringP("duration", "d", "1h", "duration to prune pulses that have missed their last checkin")
	})
	pulsesCmd.AddCommand(pulsesPruneCmd)

	return []*cobra.Command{pulsesCmd}
}

// PulseIDCompleter completes pulse IDs
func PulseIDCompleter(con *console.SliverClient) carapace.Action {
	callback := func(_ carapace.Context) carapace.Action {
		return carapace.ActionValuesDescribed(pulseCompletionPairs(con)...).Tag("pulses")
	}

	return carapace.ActionCallback(callback)
}

func registerPulseIDFlagCompletion(cmd *cobra.Command, name string, con *console.SliverClient) {
	if cmd == nil {
		return
	}
	if _, ok := cmd.GetFlagCompletionFunc(name); ok {
		return
	}
	if cmd.Flags().Lookup(name) == nil && cmd.PersistentFlags().Lookup(name) == nil {
		return
	}
	_ = cmd.RegisterFlagCompletionFunc(name, func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		values := describedValuesToTabs(pulseCompletionPairs(con))
		return filterCompletionValues(values, toComplete), cobra.ShellCompDirectiveNoFileComp
	})
}

func pulseCompletionPairs(con *console.SliverClient) []string {
	results := make([]string, 0)
	if con == nil || con.Rpc == nil {
		return results
	}

	pulses, err := con.Rpc.GetPulses(context.Background(), &commonpb.Empty{})
	if err != nil {
		return results
	}

	for _, b := range pulses.Pulses {
		link := fmt.Sprintf("[%s <- %s]", b.ActiveC2, b.RemoteAddress)
		id := fmt.Sprintf("%s (%d)", b.Name, b.PID)
		userHost := fmt.Sprintf("%s@%s", b.Username, b.Hostname)
		desc := strings.Join([]string{id, userHost, link}, " ")

		results = append(results, b.ID[:8], desc)
	}

	return results
}

func describedValuesToTabs(values []string) []string {
	tabbed := make([]string, 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		tabbed = append(tabbed, fmt.Sprintf("%s\t%s", values[i], values[i+1]))
	}
	return tabbed
}

func filterCompletionValues(values []string, prefix string) []string {
	if prefix == "" {
		return values
	}

	filtered := make([]string, 0, len(values))
	for _, value := range values {
		candidate := value
		if before, _, ok := strings.Cut(value, "\t"); ok {
			candidate = before
		}
		if strings.HasPrefix(candidate, prefix) {
			filtered = append(filtered, value)
		}
	}

	return filtered
}
