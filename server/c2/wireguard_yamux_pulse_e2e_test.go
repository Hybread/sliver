//go:build server && go_sqlite && sliver_e2e

package c2_test

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	implantWG "github.com/bishopfox/sliver/implant/sliver/transports/wireguard"
	"github.com/bishopfox/sliver/protobuf/commonpb"
	"github.com/bishopfox/sliver/protobuf/rpcpb"
	"github.com/bishopfox/sliver/protobuf/sliverpb"
	"github.com/bishopfox/sliver/server/c2"
	"github.com/bishopfox/sliver/server/transport"
	"github.com/hashicorp/yamux"
	"google.golang.org/protobuf/proto"
	uuid "uuid"
)

func TestWGYamux_Pulse_EndToEndAsyncPingRPC(t *testing.T) {
	// NOTE: If you run this test in a restricted environment where writes to
	// `~/.sliver` are blocked, set `SLIVER_ROOT_DIR` to a writable temp dir.

	grpcServer, grpcListener, err := transport.LocalListener()
	if err != nil {
		t.Fatalf("start local grpc listener: %v", err)
	}
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = grpcListener.Close()
	})

	serverConn, implantConn := net.Pipe()
	t.Cleanup(func() {
		_ = serverConn.Close()
		_ = implantConn.Close()
	})
	go c2.HandleWGSliverConnectionForTest(serverConn)

	pulseID := uuid.NewV4().String()
	pulse := startTestWGPulse(t, implantConn, pulseID)
	t.Cleanup(pulse.Stop)

	rpcConn, err := dialBufConn(context.Background(), grpcListener)
	if err != nil {
		t.Fatalf("dial grpc/bufconn: %v", err)
	}
	t.Cleanup(func() { _ = rpcConn.Close() })
	rpcClient := rpcpb.NewSliverRPCClient(rpcConn)

	waitForPulseRegistration(t, rpcClient, pulseID, 10*time.Second)

	const tasksCount = 8
	type taskInfo struct {
		id    string
		nonce int32
	}
	taskInfos := make([]taskInfo, 0, tasksCount)

	for i := 0; i < tasksCount; i++ {
		callCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		nonce := int32(1000 + i)
		resp, err := rpcClient.Ping(callCtx, &sliverpb.Ping{
			Nonce: nonce,
			Request: &commonpb.Request{
				Async:    true,
				PulseID: pulseID,
				Timeout:  int64(5 * time.Second),
			},
		})
		cancel()
		if err != nil {
			t.Fatalf("rpc ping (async): %v", err)
		}
		if resp.GetResponse() == nil || resp.GetResponse().TaskID == "" {
			t.Fatalf("expected ping task id in async response, got=%v", resp.GetResponse())
		}
		taskInfos = append(taskInfos, taskInfo{id: resp.GetResponse().TaskID, nonce: nonce})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pendingTasks, err := pulse.FetchPendingTasks(ctx)
	if err != nil {
		t.Fatalf("fetch pending pulse tasks: %v", err)
	}
	if len(pendingTasks) < tasksCount {
		t.Fatalf("unexpected pending tasks count: got=%d want>=%d", len(pendingTasks), tasksCount)
	}

	results := executePulseTasks(t, pendingTasks)
	if err := pulse.SendResults(results); err != nil {
		t.Fatalf("send pulse results: %v", err)
	}

	for _, info := range taskInfos {
		waitForPulseTaskCompleted(t, rpcClient, info.id, info.nonce, 10*time.Second)
	}
}

func TestWGYamux_Pulse_EndToEndAsyncMixedRPCs(t *testing.T) {
	// NOTE: If you run this test in a restricted environment where writes to
	// `~/.sliver` are blocked, set `SLIVER_ROOT_DIR` to a writable temp dir.

	grpcServer, grpcListener, err := transport.LocalListener()
	if err != nil {
		t.Fatalf("start local grpc listener: %v", err)
	}
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = grpcListener.Close()
	})

	serverConn, implantConn := net.Pipe()
	t.Cleanup(func() {
		_ = serverConn.Close()
		_ = implantConn.Close()
	})
	go c2.HandleWGSliverConnectionForTest(serverConn)

	pulseID := uuid.NewV4().String()
	pulse := startTestWGPulse(t, implantConn, pulseID)
	t.Cleanup(pulse.Stop)

	rpcConn, err := dialBufConn(context.Background(), grpcListener)
	if err != nil {
		t.Fatalf("dial grpc/bufconn: %v", err)
	}
	t.Cleanup(func() { _ = rpcConn.Close() })
	rpcClient := rpcpb.NewSliverRPCClient(rpcConn)

	waitForPulseRegistration(t, rpcClient, pulseID, 10*time.Second)

	testDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(testDir, "alpha.txt"), []byte("alpha"), 0600); err != nil {
		t.Fatalf("write alpha.txt: %v", err)
	}
	if err := os.Mkdir(filepath.Join(testDir, "subdir"), 0700); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	reqTimeout := int64(5 * time.Second)

	type taskCheck struct {
		id    string
		check func(*testing.T, []byte)
	}
	checks := make([]taskCheck, 0, 8)

	addTask := func(taskID string, check func(*testing.T, []byte)) {
		if taskID == "" {
			t.Fatalf("expected task id in async response")
		}
		checks = append(checks, taskCheck{id: taskID, check: check})
	}

	for i := 0; i < 4; i++ {
		callCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		nonce := int32(2000 + i)
		resp, err := rpcClient.Ping(callCtx, &sliverpb.Ping{
			Nonce: nonce,
			Request: &commonpb.Request{
				Async:    true,
				PulseID: pulseID,
				Timeout:  reqTimeout,
			},
		})
		cancel()
		if err != nil {
			t.Fatalf("rpc ping (async): %v", err)
		}

		taskID := resp.GetResponse().GetTaskID()
		wantNonce := nonce
		addTask(taskID, func(t *testing.T, data []byte) {
			t.Helper()
			ping := &sliverpb.Ping{}
			if err := proto.Unmarshal(data, ping); err != nil {
				t.Fatalf("unmarshal ping response: %v", err)
			}
			if ping.Nonce != wantNonce {
				t.Fatalf("unexpected ping nonce: got=%d want=%d", ping.Nonce, wantNonce)
			}
		})
	}

	{
		callCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		resp, err := rpcClient.Pwd(callCtx, &sliverpb.PwdReq{
			Request: &commonpb.Request{
				Async:    true,
				PulseID: pulseID,
				Timeout:  reqTimeout,
			},
		})
		cancel()
		if err != nil {
			t.Fatalf("rpc pwd (async): %v", err)
		}
		taskID := resp.GetResponse().GetTaskID()
		addTask(taskID, func(t *testing.T, data []byte) {
			t.Helper()
			pwd := &sliverpb.Pwd{}
			if err := proto.Unmarshal(data, pwd); err != nil {
				t.Fatalf("unmarshal pwd response: %v", err)
			}
			if pwd.Path == "" {
				t.Fatalf("unexpected empty pwd path")
			}
		})
	}

	{
		callCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		resp, err := rpcClient.GetEnv(callCtx, &sliverpb.EnvReq{
			Name: "PATH",
			Request: &commonpb.Request{
				Async:    true,
				PulseID: pulseID,
				Timeout:  reqTimeout,
			},
		})
		cancel()
		if err != nil {
			t.Fatalf("rpc getenv (async): %v", err)
		}
		taskID := resp.GetResponse().GetTaskID()
		addTask(taskID, func(t *testing.T, data []byte) {
			t.Helper()
			env := &sliverpb.EnvInfo{}
			if err := proto.Unmarshal(data, env); err != nil {
				t.Fatalf("unmarshal env response: %v", err)
			}
			if len(env.Variables) != 1 || env.Variables[0].Key != "PATH" {
				t.Fatalf("unexpected env response: %+v", env.Variables)
			}
		})
	}

	{
		callCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		resp, err := rpcClient.Ls(callCtx, &sliverpb.LsReq{
			Path: testDir,
			Request: &commonpb.Request{
				Async:    true,
				PulseID: pulseID,
				Timeout:  reqTimeout,
			},
		})
		cancel()
		if err != nil {
			t.Fatalf("rpc ls (async): %v", err)
		}
		taskID := resp.GetResponse().GetTaskID()
		addTask(taskID, func(t *testing.T, data []byte) {
			t.Helper()
			ls := &sliverpb.Ls{}
			if err := proto.Unmarshal(data, ls); err != nil {
				t.Fatalf("unmarshal ls response: %v", err)
			}
			if !ls.Exists {
				t.Fatalf("unexpected ls response (Exists=false): %v", ls.GetResponse())
			}
			found := false
			for _, file := range ls.Files {
				if file.Name == "alpha.txt" {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("ls missing expected alpha.txt entry")
			}
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pendingTasks, err := pulse.FetchPendingTasks(ctx)
	if err != nil {
		t.Fatalf("fetch pending pulse tasks: %v", err)
	}
	if len(pendingTasks) < len(checks) {
		t.Fatalf("unexpected pending tasks count: got=%d want>=%d", len(pendingTasks), len(checks))
	}

	results := executePulseTasks(t, pendingTasks)
	if err := pulse.SendResults(results); err != nil {
		t.Fatalf("send pulse results: %v", err)
	}

	for _, tc := range checks {
		data := waitForPulseTaskResponseBytes(t, rpcClient, tc.id, 10*time.Second)
		tc.check(t, data)
	}
}

type wgTestPulse struct {
	id       string
	conn     net.Conn
	session  *yamux.Session
	incoming chan *sliverpb.Envelope
	done     chan struct{}
}

func startTestWGPulse(t *testing.T, conn net.Conn, pulseID string) *wgTestPulse {
	t.Helper()

	if _, err := conn.Write([]byte(implantWG.YamuxPreface)); err != nil {
		t.Fatalf("write yamux preface: %v", err)
	}

	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	muxSession, err := yamux.Client(conn, cfg)
	if err != nil {
		t.Fatalf("start yamux client session: %v", err)
	}

	pulse := &wgTestPulse{
		id:       pulseID,
		conn:     conn,
		session:  muxSession,
		incoming: make(chan *sliverpb.Envelope, 64),
		done:     make(chan struct{}),
	}
	go pulse.recvLoop()

	register := &sliverpb.Register{
		Name:              "e2e-pulse",
		Hostname:          "localhost",
		Uuid:              uuid.NewV4().String(),
		Username:          "unit-test",
		Os:                runtime.GOOS,
		Arch:              runtime.GOARCH,
		Pid:               int32(os.Getpid()),
		Filename:          "sliver-e2e",
		ActiveC2:          "wg://e2e",
		Version:           "e2e",
		ReconnectInterval: 0,
		ProxyURL:          "",
		Locale:            "en_US",
	}
	regData, err := proto.Marshal(&sliverpb.PulseRegister{
		ID:          pulseID,
		Interval:    int64(1),
		Jitter:      int64(0),
		Register:    register,
		NextCheckin: int64(1),
	})
	if err != nil {
		t.Fatalf("marshal pulse register: %v", err)
	}
	if err := sendWGYamuxEnvelope(muxSession, &sliverpb.Envelope{Type: sliverpb.MsgPulseRegister, Data: regData}); err != nil {
		t.Fatalf("send pulse register: %v", err)
	}

	return pulse
}

func (b *wgTestPulse) Stop() {
	select {
	case <-b.done:
	default:
		close(b.done)
	}
	_ = b.session.Close()
	_ = b.conn.Close()
}

func (b *wgTestPulse) recvLoop() {
	defer close(b.incoming)
	for {
		stream, err := b.session.Accept()
		if err != nil {
			return
		}

		go func(stream net.Conn) {
			defer stream.Close()
			envelope, err := implantWG.ReadEnvelope(stream)
			if err != nil || envelope == nil {
				return
			}
			select {
			case b.incoming <- envelope:
			case <-b.done:
			}
		}(stream)
	}
}

func (b *wgTestPulse) FetchPendingTasks(ctx context.Context) ([]*sliverpb.Envelope, error) {
	reqData, err := proto.Marshal(&sliverpb.PulseTasks{
		ID:          b.id,
		NextCheckin: int64(1),
	})
	if err != nil {
		return nil, err
	}
	if err := sendWGYamuxEnvelope(b.session, &sliverpb.Envelope{Type: sliverpb.MsgPulseTasks, Data: reqData}); err != nil {
		return nil, err
	}

	for {
		select {
		case envelope, ok := <-b.incoming:
			if !ok {
				return nil, context.Canceled
			}
			if envelope.Type != sliverpb.MsgPulseTasks {
				continue
			}
			resp := &sliverpb.PulseTasks{}
			if err := proto.Unmarshal(envelope.Data, resp); err != nil {
				return nil, err
			}
			return resp.Tasks, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (b *wgTestPulse) SendResults(results []*sliverpb.Envelope) error {
	reqData, err := proto.Marshal(&sliverpb.PulseTasks{
		ID:          b.id,
		NextCheckin: int64(1),
		Tasks:       results,
	})
	if err != nil {
		return err
	}
	return sendWGYamuxEnvelope(b.session, &sliverpb.Envelope{Type: sliverpb.MsgPulseTasks, Data: reqData})
}
