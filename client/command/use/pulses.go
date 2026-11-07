package use

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
	"github.com/spf13/cobra"

	"github.com/bishopfox/sliver/client/command/pulses"
	"github.com/bishopfox/sliver/client/console"
)

// UsePulseCmd - Change the active pulse
func UsePulseCmd(cmd *cobra.Command, con *console.SliverClient, args []string) {
	pulse, err := pulses.SelectPulse(con)
	if pulse != nil {
		con.ActiveTarget.Set(nil, pulse)
		con.PrintInfof("Active pulse %s (%s)\n", pulse.Name, pulse.ID)
	} else if err != nil {
		switch err {
		case pulses.ErrNoPulses:
			con.PrintErrorf("No pulse available\n")
		case pulses.ErrNoSelection:
			con.PrintErrorf("No pulse selected\n")
		default:
			con.PrintErrorf("%s\n", err)
		}
	}
}
