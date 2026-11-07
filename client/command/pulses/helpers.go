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
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/bishopfox/sliver/client/console"
	"github.com/bishopfox/sliver/client/forms"
	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
)

var (
	// ErrNoPulses - No sessions available
	ErrNoPulses = errors.New("no pulses")
	// ErrNoSelection - No selection made
	ErrNoSelection = errors.New("no selection")
	// ErrPulseNotFound
	ErrPulseNotFound = errors.New("no pulse found for this ID")
)

// SelectPulse - Interactive menu for the user to select an session, optionally only display live sessions
func SelectPulse(con *console.SliverClient) (*clientpb.Pulse, error) {
	grpcCtx, cancel := con.GrpcContext(nil)
	defer cancel()
	pulses, err := con.Rpc.GetPulses(grpcCtx, &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	if len(pulses.Pulses) == 0 {
		return nil, ErrNoPulses
	}

	pulsesMap := map[string]*clientpb.Pulse{}
	for _, pulse := range pulses.Pulses {
		pulsesMap[pulse.ID] = pulse
	}
	keys := []string{}
	for pulseID := range pulsesMap {
		keys = append(keys, pulseID)
	}
	sort.Strings(keys)

	outputBuf := bytes.NewBufferString("")
	table := tabwriter.NewWriter(outputBuf, 0, 2, 2, ' ', 0)

	// Column Headers
	for _, key := range keys {
		pulse := pulsesMap[key]
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\n",
			pulse.ID,
			pulse.Name,
			pulse.RemoteAddress,
			pulse.Hostname,
			pulse.Username,
			fmt.Sprintf("%s/%s", pulse.OS, pulse.Arch),
		)
	}
	table.Flush()

	options := strings.Split(outputBuf.String(), "\n")
	options = options[:len(options)-1] // Remove the last empty option
	selected := ""
	_ = forms.Select("Select a pulse:", options, &selected)
	if selected == "" {
		return nil, ErrNoSelection
	}

	// Go from the selected option -> index -> session
	for index, option := range options {
		if option == selected {
			return pulsesMap[keys[index]], nil
		}
	}
	return nil, ErrNoSelection
}

func GetPulse(con *console.SliverClient, pulseID string) (*clientpb.Pulse, error) {
	grpcCtx, cancel := con.GrpcContext(nil)
	defer cancel()
	pulses, err := con.Rpc.GetPulses(grpcCtx, &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	if len(pulses.Pulses) == 0 {
		return nil, ErrNoPulses
	}
	for _, pulse := range pulses.Pulses {
		if pulse.ID == pulseID || strings.HasPrefix(pulse.ID, pulseID) {
			return pulse, nil
		}
	}
	return nil, ErrPulseNotFound
}

func GetPulses(con *console.SliverClient) (*clientpb.Pulses, error) {
	grpcCtx, cancel := con.GrpcContext(nil)
	defer cancel()
	pulses, err := con.Rpc.GetPulses(grpcCtx, &commonpb.Empty{})
	if err != nil {
		return nil, err
	}
	if len(pulses.Pulses) == 0 {
		return nil, ErrNoPulses
	}
	return pulses, nil
}
