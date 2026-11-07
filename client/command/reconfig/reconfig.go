package reconfig

/*
	Sliver Implant Framework
	Copyright (C) 2019  Bishop Fox

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
	"context"
	"net/url"
	"time"

	"github.com/bishopfox/sliver/client/console"
	"github.com/bishopfox/sliver/client/forms"
	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/sliverpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
)

// ReconfigCmd - Reconfigure metadata about a sessions.
func ReconfigCmd(cmd *cobra.Command, con *console.SliverClient, args []string) {
	session, pulse := con.ActiveTarget.GetInteractive()
	if session == nil && pulse == nil {
		return
	}

	var err error
	var reconnectInterval time.Duration
	interval, _ := cmd.Flags().GetString("reconnect-interval")

	if interval != "" {
		reconnectInterval, err = time.ParseDuration(interval)
		if err != nil {
			con.PrintErrorf("Invalid reconnect interval: %s\n", err)
			return
		}
	}

	var pulseInterval time.Duration
	var pulseJitter time.Duration
	binterval, _ := cmd.Flags().GetString("pulse-interval")
	bjitter, _ := cmd.Flags().GetString("pulse-jitter")
	C2URI, _ := cmd.Flags().GetString("c2-uri")

	if pulse != nil {
		if binterval != "" {
			pulseInterval, err = time.ParseDuration(binterval)
			if err != nil {
				con.PrintErrorf("Invalid pulse interval: %s\n", err)
				return
			}
		}
		if bjitter != "" {
			pulseJitter, err = time.ParseDuration(bjitter)
			if err != nil {
				con.PrintErrorf("Invalid pulse jitter: %s\n", err)
				return
			}
			if pulseInterval == 0 && pulseJitter != 0 {
				con.PrintInfof("Modified pulse jitter will take effect after next check-in\n")
			}
		}
	}

	if C2URI != "" {
		// validate uri
		newURI, err := url.Parse(C2URI)
		if err != nil || newURI.Scheme == "" || newURI.Host == "" {
			con.PrintErrorf("Invalid C2 URI: %s (expected format: protocol://host:port)\n", C2URI)
			return
		}

		// validate the protocol is the same as the pulse (cuz it compiled with only one protocol)
		var activeC2 string
		if pulse != nil {
			activeC2 = pulse.ActiveC2
		} else if session != nil {
			activeC2 = session.ActiveC2
		}

		if activeC2 != "" {
			currentURI, err := url.Parse(activeC2)
			if err == nil {
				if newURI.Scheme != currentURI.Scheme {
					confirm := false
					con.PrintWarnf("Switching protocol from %s to %s. This will only work if the protocol was included in the compiled implant. If it was NOT included, YOU WILL LOSE THE PULSE CONNECTION PERMANENTLY.\n",
						currentURI.Scheme, newURI.Scheme)
					_ = forms.Confirm("Do you want to continue?", &confirm)
					if !confirm {
						return
					}
				}
				// Warning: Switching to a different host typically fails because the implant and server must share the same crypto keys.
				if newURI.Hostname() != currentURI.Hostname() {
					con.PrintWarnf("Switching to a different host (%s -> %s). This only works if both servers share the same crypto keys.\n",
						currentURI.Hostname(), newURI.Hostname())
					confirm := false
					_ = forms.Confirm("Do you want to continue?", &confirm)
					if !confirm {
						return
					}
				}
			}
		}

		con.PrintInfof("Updating C2 endpoint to: %s\n", C2URI)
	}

	reconfig, err := con.Rpc.Reconfigure(context.Background(), &sliverpb.ReconfigureReq{
		ReconnectInterval: int64(reconnectInterval),
		PulseInterval:    int64(pulseInterval),
		PulseJitter:      int64(pulseJitter),
		C2URI:             C2URI,
		Request:           con.ActiveTarget.Request(cmd),
	})
	if err != nil {
		con.PrintWarnf("%s\n", err)
		return
	}
	if reconfig.Response != nil && reconfig.Response.Async {
		con.AddPulseCallback(reconfig.Response.TaskID, func(task *clientpb.PulseTask) {
			err = proto.Unmarshal(task.Response, reconfig)
			if err != nil {
				con.PrintErrorf("Failed to decode response %s\n", err)
				return
			}
			con.PrintInfof("Reconfigured pulse\n")
		})
		con.PrintAsyncResponse(reconfig.Response)
	} else {
		con.PrintInfof("Reconfiguration complete\n")
	}
}
