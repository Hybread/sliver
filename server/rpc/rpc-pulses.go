package rpc

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
	"context"

	"github.com/bishopfox/sliver/protobuf/clientpb"
	"github.com/bishopfox/sliver/protobuf/commonpb"
	"github.com/bishopfox/sliver/server/db"
	"github.com/bishopfox/sliver/server/db/models"
	"github.com/bishopfox/sliver/server/log"
)

var (
	pulseRpcLog = log.NamedLogger("rpc", "pulses")
)

// GetPulses - Get a list of pulses from the database
func (rpc *Server) GetPulses(ctx context.Context, req *commonpb.Empty) (*clientpb.Pulses, error) {
	pulses, err := db.ListPulses()
	if err != nil {
		pulseRpcLog.Errorf("Failed to find db pulses: %s", err)
		return nil, ErrDatabaseFailure
	}
	for id, pulse := range pulses {
		all, completed, err := db.CountTasksByPulseID(pulse.ID)
		if err != nil {
			pulseRpcLog.Errorf("Task count failed: %s", err)
		}
		pulses[id].TasksCount = all
		pulses[id].TasksCountCompleted = completed
	}
	return &clientpb.Pulses{Pulses: pulses}, nil
}

// GetPulse - Get a list of pulses from the database
func (rpc *Server) GetPulse(ctx context.Context, req *clientpb.Pulse) (*clientpb.Pulse, error) {
	pulse, err := db.PulseByID(req.ID)
	if err != nil {
		pulseRpcLog.Error(err)
		return nil, ErrDatabaseFailure
	}
	return pulse.ToProtobuf(), nil
}

// RmPulse - Delete a pulse and any related tasks
func (rpc *Server) RmPulse(ctx context.Context, req *clientpb.Pulse) (*commonpb.Empty, error) {
	pulse, err := db.PulseByID(req.ID)
	if err != nil {
		pulseRpcLog.Error(err)
		return nil, ErrInvalidPulseID
	}

	err = db.Session().Where(&models.PulseTask{
		PulseID: pulse.ID},
	).Delete(&models.PulseTask{}).Error
	if err != nil {
		pulseRpcLog.Errorf("Database error: %s", err)
		return nil, ErrDatabaseFailure
	}
	err = db.Session().Delete(pulse).Error
	if err != nil {
		pulseRpcLog.Errorf("Database error: %s", err)
		return nil, ErrDatabaseFailure
	}
	return &commonpb.Empty{}, nil
}

// GetPulseTasks - Get a list of tasks for a specific pulse
func (rpc *Server) GetPulseTasks(ctx context.Context, req *clientpb.Pulse) (*clientpb.PulseTasks, error) {
	pulse, err := db.PulseByID(req.ID)
	if err != nil {
		return nil, ErrInvalidPulseID
	}
	tasks, err := db.PulseTasksByPulseID(pulse.ID.String())
	if err != nil {
		return nil, rpcError(err)
	}
	return &clientpb.PulseTasks{Tasks: tasks}, nil
}

// GetPulseTaskContent - Get the content of a specific task
func (rpc *Server) GetPulseTaskContent(ctx context.Context, req *clientpb.PulseTask) (*clientpb.PulseTask, error) {
	task, err := db.PulseTaskByID(req.ID)
	if err != nil {
		return nil, ErrInvalidPulseTaskID
	}
	return task, nil
}

// CancelPulseTask - Cancel a pulse task
func (rpc *Server) CancelPulseTask(ctx context.Context, req *clientpb.PulseTask) (*clientpb.PulseTask, error) {
	task, err := db.PulseTaskByID(req.ID)
	if err != nil {
		return nil, ErrInvalidPulseTaskID
	}
	if task.State == models.PENDING {
		err = db.Session().Model(&models.PulseTask{}).
			Where("id=?", task.ID).
			Updates(map[string]interface{}{"state": models.CANCELED}).
			Error
		if err != nil {
			pulseRpcLog.Errorf("Database error: %s", err)
			return nil, ErrDatabaseFailure
		}
	} else {
		// No real point to cancel the task if it's already been sent
		return task, ErrInvalidPulseTaskCancelState
	}
	task, err = db.PulseTaskByID(req.ID)
	if err != nil {
		return nil, ErrInvalidPulseTaskID
	}
	return task, nil
}

// UpdatePulseIntegrityInformation - Update process integrity information for a pulse
func (rpc *Server) UpdatePulseIntegrityInformation(ctx context.Context, req *clientpb.PulseIntegrity) (*commonpb.Empty, error) {
	resp := &commonpb.Empty{}
	pulse, err := db.PulseByID(req.PulseID)
	if err != nil || pulse == nil {
		return resp, ErrInvalidPulseID
	}
	pulse.Integrity = req.Integrity
	err = db.Session().Save(pulse).Error
	if err != nil {
		return resp, rpcError(err)
	}
	return resp, nil
}
