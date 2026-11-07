package rpc

/*
	Sliver Implant Framework
	Copyright (C) 2022  Bishop Fox

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

	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
	"github.com/bishopfox/sliver/protobuf/sliverpb"
	"github.com/bishopfox/sliver/server/core"
	"github.com/bishopfox/sliver/server/db"
	"github.com/bishopfox/sliver/util"
)

const maxNameLength = 32

// Reconfigure - Reconfigure a pulse/session
func (rpc *Server) Reconfigure(ctx context.Context, req *sliverpb.ReconfigureReq) (*sliverpb.Reconfigure, error) {
	if req == nil || req.Request == nil {
		return nil, ErrMissingRequestField
	}

	// We have to preserve these because GenericHandler clears them in req.Request
	sessionID := req.Request.SessionID
	pulseID := req.Request.PulseID

	resp := &sliverpb.Reconfigure{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}

	// Successfully execute command, update server's info on reconnect interval
	if sessionID != "" {
		session := core.Sessions.Get(sessionID)
		if session == nil {
			return nil, ErrInvalidSessionID
		}
		if req.ReconnectInterval != 0 {
			session.ReconnectInterval = req.ReconnectInterval
		}
		core.Sessions.UpdateSession(session)
	} else if pulseID != "" {
		pulse, err := db.PulseByID(pulseID)
		if err != nil || pulse == nil {
			return nil, ErrInvalidPulseID
		}
		if req.PulseInterval != 0 {
			pulse.Interval = req.PulseInterval
		}
		if req.PulseJitter != 0 {
			pulse.Jitter = req.PulseJitter
		}
		err = db.Session().Save(pulse).Error
		if err != nil {
			return nil, rpcError(err)
		}
	} else {
		return nil, ErrMissingRequestField
	}
	return resp, nil
}

// Rename - Rename a pulse/session
func (rpc *Server) Rename(ctx context.Context, req *clientpb.RenameReq) (*commonpb.Empty, error) {
	resp := &commonpb.Empty{}

	if len(req.Name) < 1 || maxNameLength < len(req.Name) {
		return resp, ErrInvalidName
	}
	if err := util.AllowedName(req.Name); err != nil {
		return resp, ErrInvalidName
	}

	if req.SessionID != "" {
		session := core.Sessions.Get(req.SessionID)
		if session == nil {
			return nil, ErrInvalidSessionID
		}
		session.Name = req.Name
	} else if req.PulseID != "" {
		pulse, err := db.PulseByID(req.PulseID)
		if err != nil || pulse == nil {
			return nil, ErrInvalidPulseID
		}
		err = db.RenamePulse(pulse.ID.String(), req.Name)
		if err != nil {
			return nil, rpcError(err)
		}
	} else {
		return nil, ErrMissingRequestField
	}
	return resp, nil
}
