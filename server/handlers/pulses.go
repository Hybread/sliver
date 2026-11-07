package handlers

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
	------------------------------------------------------------------------

	WARNING: These functions can be invoked by remote implants without user interaction

*/

import (
	"encoding/json"
	"errors"
	"time"

	consts "github.com/bishopfox/sliver/client/constants"
	"github.com/bishopfox/sliver/protobuf/clientpb"
	sliverpb "github.com/bishopfox/sliver/protobuf/sliverpb"
	"github.com/bishopfox/sliver/server/core"
	"github.com/bishopfox/sliver/server/db"
	"github.com/bishopfox/sliver/server/db/models"
	"github.com/bishopfox/sliver/server/log"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

var (
	pulseHandlerLog = log.NamedLogger("handlers", "pulses")
)

func pulseRegisterHandler(implantConn *core.ImplantConnection, data []byte) *sliverpb.Envelope {
	pulseReg := &sliverpb.PulseRegister{}
	err := proto.Unmarshal(data, pulseReg)
	if err != nil {
		pulseHandlerLog.Errorf("Error decoding pulse registration message: %s", err)
		return nil
	}
	pulseHandlerLog.Infof("Pulse registration from %s", pulseReg.ID)
	pulse, err := db.PulseByID(pulseReg.ID)
	pulseHandlerLog.Debugf("Found %v err = %s", pulse, err)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		pulseHandlerLog.Errorf("Database query error %s", err)
		return nil
	}
	pulseUUID, _ := models.ParseUUID(pulseReg.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		pulse = &models.Pulse{
			ID: pulseUUID,
		}
	}
	pulseRegUUID, _ := models.ParseUUID(pulseReg.Register.Uuid)
	pulse.Name = pulseReg.Register.Name
	pulse.Hostname = pulseReg.Register.Hostname
	pulse.UUID = pulseRegUUID
	pulse.Username = pulseReg.Register.Username
	pulse.UID = pulseReg.Register.Uid
	pulse.GID = pulseReg.Register.Gid
	pulse.OS = pulseReg.Register.Os
	pulse.Arch = pulseReg.Register.Arch
	pulse.Transport = implantConn.Transport
	pulse.RemoteAddress = implantConn.RemoteAddress
	pulse.PID = pulseReg.Register.Pid
	pulse.Filename = pulseReg.Register.Filename
	pulse.LastCheckin = implantConn.GetLastMessage()
	pulse.Version = pulseReg.Register.Version
	pulse.ReconnectInterval = pulseReg.Register.ReconnectInterval
	pulse.ActiveC2 = pulseReg.Register.ActiveC2
	pulse.ProxyURL = pulseReg.Register.ProxyURL
	// pulse.ConfigID = models.ParseUUIDOrNil(pulseReg.Register.ConfigID)
	pulse.Locale = pulseReg.Register.Locale
	pulse.Capabilities = pulseReg.Register.Capabilities

	pulse.Interval = pulseReg.Interval
	pulse.Jitter = pulseReg.Jitter
	pulse.NextCheckin = time.Now().Unix() + pulseReg.NextCheckin

	err = db.Session().Save(pulse).Error
	if err != nil {
		pulseHandlerLog.Errorf("Database write %s", err)
	}

	eventData, _ := proto.Marshal(pulse.ToProtobuf())
	core.EventBroker.Publish(core.Event{
		EventType: consts.PulseRegisteredEvent,
		Data:      eventData,
		Pulse:    pulse,
	})

	go auditLogPulse(pulse, pulseReg.Register)
	return nil
}

type auditLogNewPulseMsg struct {
	Pulse   *clientpb.Pulse
	Register *sliverpb.Register
}

func auditLogPulse(pulse *models.Pulse, register *sliverpb.Register) {
	msg, err := json.Marshal(auditLogNewPulseMsg{
		Pulse:   pulse.ToProtobuf(),
		Register: register,
	})
	if err != nil {
		pulseHandlerLog.Errorf("Failed to log new pulse to audit log: %s", err)
	} else {
		log.AuditLogger.Warn(string(msg))
	}
}

func pulseTasksHandler(implantConn *core.ImplantConnection, data []byte) *sliverpb.Envelope {
	pulseTasks := &sliverpb.PulseTasks{}
	err := proto.Unmarshal(data, pulseTasks)
	if err != nil {
		pulseHandlerLog.Errorf("Error decoding pulse tasks message: %s", err)
		return nil
	}
	go func() {
		err := db.UpdatePulseCheckinByID(pulseTasks.ID, pulseTasks.NextCheckin)
		if err != nil {
			pulseHandlerLog.Errorf("failed to update checkin: %s", err)
		}
	}()

	// If the message contains tasks then process it as results
	// otherwise send the pulse any pending tasks. Currently we
	// don't receive results and send pending tasks at the same
	// time. We only send pending tasks if the request is empty.
	// If we send the Pulse 0 tasks it should not respond at all.
	if 0 < len(pulseTasks.Tasks) {
		pulseHandlerLog.Infof("Pulse %s returned %d task result(s)", pulseTasks.ID, len(pulseTasks.Tasks))
		go pulseTaskResults(pulseTasks.ID, pulseTasks.Tasks)
		return nil
	}

	pulseHandlerLog.Infof("Pulse %s requested pending task(s)", pulseTasks.ID)

	// Pending tasks are ordered by their creation time.
	pendingTasks, err := db.PendingPulseTasksByPulseID(pulseTasks.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		pulseHandlerLog.Errorf("Pulse task database error: %s", err)
		return nil
	}
	tasks := []*sliverpb.Envelope{}
	for _, pendingTask := range pendingTasks {
		envelope := &sliverpb.Envelope{}
		err = proto.Unmarshal(pendingTask.Request, envelope)
		if err != nil {
			pulseHandlerLog.Errorf("Error decoding pending task: %s", err)
			continue
		}
		envelope.ID = pendingTask.EnvelopeID
		tasks = append(tasks, envelope)
		pendingTask.State = models.SENT
		pendingTask.SentAt = time.Now().Unix()
		err = db.Session().Model(&models.PulseTask{}).Where(&models.PulseTask{
			ID: pendingTask.ID,
		}).Updates(pendingTask).Error
		if err != nil {
			pulseHandlerLog.Errorf("Database error: %s", err)
		}
	}
	taskData, err := proto.Marshal(&sliverpb.PulseTasks{Tasks: tasks})
	if err != nil {
		pulseHandlerLog.Errorf("Error marshaling pulse tasks message: %s", err)
		return nil
	}
	pulseHandlerLog.Infof("Sending %d task(s) to pulse %s", len(pendingTasks), pulseTasks.ID)
	return &sliverpb.Envelope{
		Type: sliverpb.MsgPulseTasks,
		Data: taskData,
	}
}

func pulseTaskResults(pulseID string, taskEnvelopes []*sliverpb.Envelope) *sliverpb.Envelope {
	for _, envelope := range taskEnvelopes {
		dbTask, err := db.PulseTaskByEnvelopeID(pulseID, envelope.ID)
		if err != nil {
			pulseHandlerLog.Errorf("Error finding db task: %s", err)
			continue
		}
		if dbTask == nil {
			pulseHandlerLog.Errorf("Error: nil db task!")
			continue
		}
		dbTask.State = models.COMPLETED
		dbTask.CompletedAt = time.Now().Unix()
		dbTask.Response = envelope.Data
		id, _ := models.ParseUUID(dbTask.ID)
		err = db.Session().Model(&models.PulseTask{}).Where(&models.PulseTask{
			ID: id,
		}).Updates(dbTask).Error
		if err != nil {
			pulseHandlerLog.Errorf("Error updating db task: %s", err)
			continue
		}
		eventData, _ := proto.Marshal(dbTask)
		core.EventBroker.Publish(core.Event{
			EventType: consts.PulseTaskResultEvent,
			Data:      eventData,
		})
	}
	return nil
}
