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
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/bishopfox/sliver/client/command/kill"
	"github.com/bishopfox/sliver/client/command/settings"
	"github.com/bishopfox/sliver/client/command/targettable"
	"github.com/bishopfox/sliver/client/console"
	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
)

// PulsesCmd - Display/interact with pulses
func PulsesCmd(cmd *cobra.Command, con *console.SliverClient, args []string) {
	killFlag, _ := cmd.Flags().GetString("kill")
	killAll, _ := cmd.Flags().GetBool("kill-all")
	interact, _ := cmd.Flags().GetString("interact")

	// Handle kill
	if killFlag != "" {
		pulse, err := GetPulse(con, killFlag)
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
		err = kill.KillPulse(pulse, cmd, con)
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
		con.Println()
		con.PrintInfof("Killed %s (%s)\n", pulse.Name, pulse.ID)
	}

	if killAll {
		pulses, err := GetPulses(con)
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
		for _, pulse := range pulses.Pulses {
			err = kill.KillPulse(pulse, cmd, con)
			if err != nil {
				con.PrintErrorf("%s\n", err)
				return
			}
			con.Println()
			con.PrintInfof("Killed %s (%s)\n", pulse.Name, pulse.ID)
		}
	}

	if interact != "" {
		pulse, err := GetPulse(con, interact)
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
		con.ActiveTarget.Set(nil, pulse)
		con.PrintInfof("Active pulse %s (%s)\n", pulse.Name, strings.Split(pulse.ID, "-")[0])
		return
	}

	filter, _ := cmd.Flags().GetString("filter")
	var filterRegex *regexp.Regexp
	if filterRe, _ := cmd.Flags().GetString("filter-re"); filterRe != "" {
		var err error
		filterRegex, err = regexp.Compile(filterRe)
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
	}

	grpcCtx, cancel := con.GrpcContext(cmd)
	defer cancel()
	pulses, err := con.Rpc.GetPulses(grpcCtx, &commonpb.Empty{})
	if err != nil {
		con.PrintErrorf("%s\n", err)
		return
	}
	PrintPulses(pulses.Pulses, filter, filterRegex, con)
}

// PrintPulses - Display a list of pulses
func PrintPulses(pulses []*clientpb.Pulse, filter string, filterRegex *regexp.Regexp, con *console.SliverClient) {
	if len(pulses) == 0 {
		con.PrintInfof("No pulses 🙁\n")
		return
	}
	tw := renderPulses(pulses, filter, filterRegex, con)
	con.Printf("%s\n", tw.Render())
}

func renderPulses(pulses []*clientpb.Pulse, filter string, filterRegex *regexp.Regexp, con *console.SliverClient) table.Writer {
	width, _, err := term.GetSize(0)
	if err != nil {
		width = 999
	}
	return renderPulsesAtWidth(pulses, filter, filterRegex, con, width, time.Now())
}

// renderPulsesAtWidth uses one reference time for all candidate layouts,
// preventing a second boundary from changing the table width while it is fitted.
func renderPulsesAtWidth(pulses []*clientpb.Pulse, filter string, filterRegex *regexp.Regexp, con *console.SliverClient, width int, now time.Time) table.Writer {
	windowsPulseInList := false
	for _, pulse := range pulses {
		if pulse.OS == "windows" {
			windowsPulseInList = true
			break
		}
	}

	return targettable.Fit(width, windowsPulseInList, func(layout targettable.Layout) table.Writer {
		return buildPulseTable(pulses, filter, filterRegex, con, layout, windowsPulseInList, now)
	})
}

func buildPulseTable(pulses []*clientpb.Pulse, filter string, filterRegex *regexp.Regexp, con *console.SliverClient, layout targettable.Layout, windowsPulseInList bool, now time.Time) table.Writer {
	tw := table.NewWriter()
	tw.SetStyle(settings.GetTableStyle(con))
	includeIntegrity := windowsPulseInList && layout.IncludeIntegrity
	if layout.Compact {
		tw.AppendHeader(table.Row{
			"ID",
			"Name",
			"Transport",
			"Hostname",
			"Username",
			"Operating System",
			"Last Check-in",
			"Next Check-in",
		})
	} else {
		header := table.Row{
			"ID",
			"Name",
			"Tasks",
			"Transport",
			"Remote Address",
			"Hostname",
			"Username",
			"Process (PID)",
		}
		if includeIntegrity {
			header = append(header, "Integrity")
		}
		header = append(header, "Operating System")
		if layout.IncludeLocale {
			header = append(header, "Locale")
		}
		header = append(header, "Last Check-in", "Next Check-in")
		tw.AppendHeader(header)
	}

	activePulseID := ""
	if con.ActiveTarget != nil {
		if activePulse := con.ActiveTarget.GetPulse(); activePulse != nil {
			activePulseID = activePulse.ID
		}
	}

	for _, pulse := range pulses {
		integrity := pulse.Integrity
		if integrity == "" {
			integrity = "-"
		}
		username := strings.TrimPrefix(pulse.Username, pulse.Hostname+"\\")
		lastCheckin := time.Unix(pulse.LastCheckin, 0)
		nextCheckin := time.Unix(pulse.NextCheckin, 0)
		canonicalEntries := []string{
			strings.Split(pulse.ID, "-")[0],
			pulse.Name,
			fmt.Sprintf("%d/%d", pulse.TasksCountCompleted, pulse.TasksCount),
			pulse.Transport,
			pulse.RemoteAddress,
			pulse.Hostname,
			username,
			fmt.Sprintf("%s (%d)", pulse.Filename, pulse.PID),
			integrity,
			fmt.Sprintf("%s/%s", pulse.OS, pulse.Arch),
			pulse.Locale,
			formatPulseDateDelta(lastCheckin, now, true, false),
			formatPulseDateDelta(nextCheckin, now, true, false),
		}
		if !pulseMatchesFilter(canonicalEntries, filter, filterRegex) {
			continue
		}

		style := console.StyleNormal
		if activePulseID == pulse.ID {
			style = console.StyleGreen
		}

		var rowEntries []string
		if layout.Compact {
			rowEntries = []string{
				style.Render(strings.Split(pulse.ID, "-")[0]),
				style.Render(pulse.Name),
				style.Render(pulse.Transport),
				style.Render(pulse.Hostname),
				style.Render(username),
				style.Render(fmt.Sprintf("%s/%s", pulse.OS, pulse.Arch)),
				formatPulseDateDelta(lastCheckin, now, false, false),
				formatPulseDateDelta(nextCheckin, now, false, true),
			}
		} else {
			processName := pulse.Filename
			if layout.ProcessBasename {
				processName = targettable.ProcessName(processName)
			}
			rowEntries = []string{
				style.Render(strings.Split(pulse.ID, "-")[0]),
				style.Render(pulse.Name),
				style.Render(fmt.Sprintf("%d/%d", pulse.TasksCountCompleted, pulse.TasksCount)),
				style.Render(pulse.Transport),
				style.Render(pulse.RemoteAddress),
				style.Render(pulse.Hostname),
				style.Render(username),
				style.Render(fmt.Sprintf("%s (%d)", processName, pulse.PID)),
			}
			if includeIntegrity {
				rowEntries = append(rowEntries, style.Render(integrity))
			}
			rowEntries = append(rowEntries, style.Render(fmt.Sprintf("%s/%s", pulse.OS, pulse.Arch)))
			if layout.IncludeLocale {
				rowEntries = append(rowEntries, style.Render(pulse.Locale))
			}
			rowEntries = append(rowEntries,
				formatPulseDateDelta(lastCheckin, now, true, false),
				formatPulseDateDelta(nextCheckin, now, true, true),
			)
		}

		row := table.Row{}
		for _, entry := range rowEntries {
			row = append(row, entry)
		}
		tw.AppendRow(row)
	}
	return tw
}

func pulseMatchesFilter(entries []string, filter string, filterRegex *regexp.Regexp) bool {
	if filter == "" && filterRegex == nil {
		return true
	}
	for _, entry := range entries {
		if filter != "" && strings.Contains(entry, filter) {
			return true
		}
		if filterRegex != nil && filterRegex.MatchString(entry) {
			return true
		}
	}
	return false
}

func formatPulseDateDelta(timestamp time.Time, now time.Time, includeDate bool, color bool) string {
	date := timestamp.Format(time.UnixDate)
	interval := ""
	if timestamp.Before(now) {
		delta := now.Sub(timestamp).Round(time.Second)
		if includeDate {
			interval = fmt.Sprintf("%s (%s ago)", date, delta)
		} else {
			interval = delta.String()
		}
		if color {
			interval = console.StyleBoldRed.Render(interval)
		}
	} else {
		delta := timestamp.Sub(now).Round(time.Second)
		if includeDate {
			interval = fmt.Sprintf("%s (in %s)", date, delta)
		} else {
			interval = delta.String()
		}
		if color {
			interval = console.StyleBoldGreen.Render(interval)
		}
	}
	return interval
}
