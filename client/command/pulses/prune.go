package pulses

/*
	Sliver Implant Framework
	Copyright (C) 2021  Bishop Fox

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/bishopfox/sliver/client/console"
	"github.com/bishopfox/sliver/client/forms"
	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
)

// PulsesPruneCmd - Prune stale pulses automatically
func PulsesPruneCmd(cmd *cobra.Command, con *console.SliverClient, args []string) {
	duration, _ := cmd.Flags().GetString("duration")
	pruneDuration, err := time.ParseDuration(duration)
	if err != nil {
		con.PrintErrorf("%s\n", err)
		return
	}
	con.PrintInfof("Pruning pulses that missed their last checking by %s or more...\n\n", pruneDuration)
	grpcCtx, cancel := con.GrpcContext(cmd)
	defer cancel()
	pulses, err := con.Rpc.GetPulses(grpcCtx, &commonpb.Empty{})
	if err != nil {
		con.PrintErrorf("%s\n", err)
		return
	}
	prunePulses := []*clientpb.Pulse{}
	for _, pulse := range pulses.Pulses {
		nextCheckin := time.Unix(pulse.NextCheckin, 0)
		if time.Now().Before(nextCheckin) {
			continue
		}
		delta := time.Since(nextCheckin)
		if pruneDuration <= delta {
			prunePulses = append(prunePulses, pulse)
		}
	}
	if len(prunePulses) == 0 {
		con.PrintInfof("No pulses to prune.\n")
		return
	}
	con.PrintWarnf("The following pulses and their tasks will be removed:\n")
	for index, pulse := range prunePulses {
		pulse, err := con.Rpc.GetPulse(grpcCtx, &clientpb.Pulse{ID: pulse.ID})
		if err != nil {
			con.PrintErrorf("%s\n", err)
			continue
		}
		con.Printf("\t%d. %s (%s)\n", (index + 1), pulse.Name, pulse.ID)
	}
	con.Println()
	confirm := false
	_ = forms.Confirm("Prune these pulses?", &confirm)
	if !confirm {
		return
	}
	errCount := 0
	for _, pulse := range prunePulses {
		_, err := con.Rpc.RmPulse(grpcCtx, &clientpb.Pulse{ID: pulse.ID})
		if err != nil {
			con.PrintErrorf("%s\n", err)
			errCount++
		}
	}
	con.PrintInfof("Pruned %d pulse(s)\n", len(prunePulses)-errCount)
}
