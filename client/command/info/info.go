package info

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
	"time"

	"github.com/bishopfox/sliver/client/command/use"
	"github.com/bishopfox/sliver/client/console"
	consts "github.com/bishopfox/sliver/client/constants"
	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/sliverpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
)

// InfoCmd - Display information about the active session.
func InfoCmd(cmd *cobra.Command, con *console.SliverClient, args []string) {
	var err error

	// Check if we have an active target via 'use'
	session, pulse := con.ActiveTarget.Get()

	if len(args) > 0 {
		// ID passed via argument takes priority
		idArg := args[0]
		session, pulse, err = use.SessionOrPulseByID(idArg, con)
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
	} else if session != nil || pulse != nil {
		currID := ""
		if session != nil {
			currID = session.ID
		} else {
			currID = pulse.ID
		}
		session, pulse, err = use.SessionOrPulseByID(currID, con)
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
	} else {
		if session == nil && pulse == nil {
			session, pulse, err = use.SelectSessionOrPulse(con)
			if err != nil {
				con.PrintErrorf("%s\n", err)
				return
			}
		}
	}

	if session != nil {

		con.Printf("%s %s\n", console.StyleBold.Render("        Session ID:"), session.ID)
		con.Printf("%s %s\n", console.StyleBold.Render("              Name:"), session.Name)
		con.Printf("%s %s\n", console.StyleBold.Render("          Hostname:"), session.Hostname)
		con.Printf("%s %s\n", console.StyleBold.Render("              UUID:"), session.UUID)
		con.Printf("%s %s\n", console.StyleBold.Render("          Username:"), session.Username)
		con.Printf("%s %s\n", console.StyleBold.Render("               UID:"), session.UID)
		con.Printf("%s %s\n", console.StyleBold.Render("               GID:"), session.GID)
		con.Printf("%s %d\n", console.StyleBold.Render("               PID:"), session.PID)
		con.Printf("%s %s\n", console.StyleBold.Render("                OS:"), session.OS)
		con.Printf("%s %s\n", console.StyleBold.Render("           Version:"), session.Version)
		con.Printf("%s %s\n", console.StyleBold.Render("            Locale:"), session.Locale)
		con.Printf("%s %s\n", console.StyleBold.Render("              Arch:"), session.Arch)
		con.Printf("%s %s\n", console.StyleBold.Render("         Active C2:"), session.ActiveC2)
		con.Printf("%s %s\n", console.StyleBold.Render("    Remote Address:"), session.RemoteAddress)
		con.Printf("%s %s\n", console.StyleBold.Render("         Proxy URL:"), session.ProxyURL)
		con.Printf("%s %s\n", console.StyleBold.Render("Reconnect Interval:"), time.Duration(session.ReconnectInterval).String())
		con.Printf("%s %s\n", console.StyleBold.Render("     First Contact:"), con.FormatDateDelta(time.Unix(session.FirstContact, 0), true, false))
		con.Printf("%s %s\n", console.StyleBold.Render("      Last Checkin:"), con.FormatDateDelta(time.Unix(session.LastCheckin, 0), true, false))

	} else if pulse != nil {

		con.Printf("%s %s\n", console.StyleBold.Render("         Pulse ID:"), pulse.ID)
		con.Printf("%s %s\n", console.StyleBold.Render("              Name:"), pulse.Name)
		con.Printf("%s %s\n", console.StyleBold.Render("          Hostname:"), pulse.Hostname)
		con.Printf("%s %s\n", console.StyleBold.Render("              UUID:"), pulse.UUID)
		con.Printf("%s %s\n", console.StyleBold.Render("          Username:"), pulse.Username)
		con.Printf("%s %s\n", console.StyleBold.Render("               UID:"), pulse.UID)
		con.Printf("%s %s\n", console.StyleBold.Render("               GID:"), pulse.GID)
		con.Printf("%s %d\n", console.StyleBold.Render("               PID:"), pulse.PID)
		con.Printf("%s %s\n", console.StyleBold.Render("                OS:"), pulse.OS)
		con.Printf("%s %s\n", console.StyleBold.Render("           Version:"), pulse.Version)
		con.Printf("%s %s\n", console.StyleBold.Render("            Locale:"), pulse.Locale)
		con.Printf("%s %s\n", console.StyleBold.Render("              Arch:"), pulse.Arch)
		con.Printf("%s %s\n", console.StyleBold.Render("         Active C2:"), pulse.ActiveC2)
		con.Printf("%s %s\n", console.StyleBold.Render("    Remote Address:"), pulse.RemoteAddress)
		con.Printf("%s %s\n", console.StyleBold.Render("         Proxy URL:"), pulse.ProxyURL)
		con.Printf("%s %s\n", console.StyleBold.Render("          Interval:"), time.Duration(pulse.Interval).String())
		con.Printf("%s %s\n", console.StyleBold.Render("            Jitter:"), time.Duration(pulse.Jitter).String())
		con.Printf("%s %s\n", console.StyleBold.Render("     First Contact:"), con.FormatDateDelta(time.Unix(pulse.FirstContact, 0), true, false))
		con.Printf("%s %s\n", console.StyleBold.Render("      Last Checkin:"), con.FormatDateDelta(time.Unix(pulse.LastCheckin, 0), true, false))
		con.Printf("%s %s\n", console.StyleBold.Render("      Next Checkin:"), con.FormatDateDelta(time.Unix(pulse.NextCheckin, 0), true, true))

	} else {
		con.PrintErrorf("No target session, see `help %s`\n", consts.InfoStr)
	}
}

// PIDCmd - Get the active session's PID.
func PIDCmd(cmd *cobra.Command, con *console.SliverClient, args []string) {
	session, pulse := con.ActiveTarget.GetInteractive()
	if session == nil && pulse == nil {
		return
	}
	if session != nil {
		con.Printf("%d\n", session.PID)
	} else if pulse != nil {
		con.Printf("%d\n", pulse.PID)
	}
}

// UIDCmd - Get the active session's UID.
func UIDCmd(cmd *cobra.Command, con *console.SliverClient, args []string) {
	session, pulse := con.ActiveTarget.GetInteractive()
	if session == nil && pulse == nil {
		return
	}
	if session != nil {
		con.Printf("%s\n", session.UID)
	} else if pulse != nil {
		con.Printf("%s\n", pulse.UID)
	}
}

// GIDCmd - Get the active session's GID.
func GIDCmd(cmd *cobra.Command, con *console.SliverClient, args []string) {
	session, pulse := con.ActiveTarget.GetInteractive()
	if session == nil && pulse == nil {
		return
	}
	if session != nil {
		con.Printf("%s\n", session.GID)
	} else if pulse != nil {
		con.Printf("%s\n", pulse.GID)
	}
}

// WhoamiCmd - Displays the current user of the active session.
func WhoamiCmd(cmd *cobra.Command, con *console.SliverClient, args []string) {
	session, pulse := con.ActiveTarget.GetInteractive()
	if session == nil && pulse == nil {
		return
	}

	var isWin bool
	con.Printf("Logon ID: ")
	if session != nil {
		con.Printf("%s\n", session.Username)
		if session.GetOS() == "windows" {
			isWin = true
		}
	} else if pulse != nil {
		con.Printf("%s\n", pulse.Username)
		if pulse.GetOS() == "windows" {
			isWin = true
		}
	}

	if isWin {
		cto, err := con.Rpc.CurrentTokenOwner(context.Background(), &sliverpb.CurrentTokenOwnerReq{
			Request: con.ActiveTarget.Request(cmd),
		})
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}

		if cto.Response != nil && cto.Response.Async {
			con.AddPulseCallback(cto.Response.TaskID, func(task *clientpb.PulseTask) {
				err = proto.Unmarshal(task.Response, cto)
				if err != nil {
					con.PrintErrorf("Failed to decode response %s\n", err)
					return
				}
				PrintTokenOwner(cto, con)
			})
			con.PrintAsyncResponse(cto.Response)
		} else {
			PrintTokenOwner(cto, con)
		}
	}
}

func PrintTokenOwner(cto *sliverpb.CurrentTokenOwner, con *console.SliverClient) {
	if cto.Response != nil && cto.Response.Err != "" {
		con.PrintErrorf("%s\n", cto.Response.Err)
		return
	}
	con.PrintInfof("Current Token ID: %s", cto.Output)
}
