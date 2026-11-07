//go:build client

package opfor

import (
	"context"
	"fmt"
	"strings"

	"github.com/bishopfox/sliver/protobuf/commonpb"
	opforengine "github.com/sliverarmory/opfor"
)

func (manager *Manager) querySession(
	ctx context.Context,
	query opforengine.AggressorSessionQuery,
) (opforengine.Value, error) {
	if err := ctx.Err(); err != nil {
		return opforengine.Null(), err
	}

	switch query.Kind {
	case opforengine.AggressorSessionQueryBeacons,
		opforengine.AggressorSessionQueryBeaconIDs:
		return manager.queryAllSessions(ctx, query.Kind)
	default:
		return manager.querySingleSession(ctx, query)
	}
}

func (manager *Manager) queryAllSessions(
	ctx context.Context,
	kind opforengine.AggressorSessionQueryKind,
) (opforengine.Value, error) {
	targets, err := manager.allTargets(ctx)
	if err != nil {
		return opforengine.Null(), err
	}
	values := make([]opforengine.Value, 0, len(targets))
	for _, target := range targets {
		if kind == opforengine.AggressorSessionQueryBeaconIDs {
			values = append(values, opforengine.String(target.id()))
		} else {
			values = append(values, opforengine.HashValue(targetMetadata(target)))
		}
	}
	return opforengine.ArrayValue(opforengine.NewArray(values...)), nil
}

func (manager *Manager) querySingleSession(
	ctx context.Context,
	query opforengine.AggressorSessionQuery,
) (opforengine.Value, error) {
	target, err := manager.resolveTarget(ctx, query.SessionID.String())
	if err != nil {
		return opforengine.Null(), err
	}
	metadata := targetMetadata(target)
	switch query.Kind {
	case opforengine.AggressorSessionQueryBeaconArchitecture,
		opforengine.AggressorSessionQueryBeaconData,
		opforengine.AggressorSessionQueryBeaconInfo,
		opforengine.AggressorSessionQueryIs64:
		return querySessionMetadata(target, metadata, query)
	case opforengine.AggressorSessionQueryIsActive,
		opforengine.AggressorSessionQueryIsAdmin,
		opforengine.AggressorSessionQueryIsBeacon,
		opforengine.AggressorSessionQueryIsSSH:
		return querySessionStatus(target, query)
	default:
		return opforengine.Null(), fmt.Errorf("opfor: unsupported session query %q", query.Name)
	}
}

func querySessionMetadata(
	target resolvedTarget,
	metadata *opforengine.Hash,
	query opforengine.AggressorSessionQuery,
) (opforengine.Value, error) {
	switch query.Kind {
	case opforengine.AggressorSessionQueryBeaconArchitecture:
		architecture, err := pulseArchitecture(target.arch())
		if err != nil {
			return opforengine.Null(), err
		}
		return opforengine.String(architecture), nil
	case opforengine.AggressorSessionQueryBeaconData:
		return opforengine.HashValue(metadata), nil
	case opforengine.AggressorSessionQueryBeaconInfo:
		value, found := metadata.Get(strings.ToLower(query.Key.String()))
		if !found {
			return opforengine.Null(), nil
		}
		return value, nil
	case opforengine.AggressorSessionQueryIs64:
		architecture, err := pulseArchitecture(target.arch())
		if err != nil {
			return opforengine.Null(), err
		}
		return opforengine.Bool(architecture == "x64"), nil
	default:
		return opforengine.Null(), fmt.Errorf("opfor: unsupported session metadata query %q", query.Name)
	}
}

func querySessionStatus(
	target resolvedTarget,
	query opforengine.AggressorSessionQuery,
) (opforengine.Value, error) {
	switch query.Kind {
	case opforengine.AggressorSessionQueryIsActive:
		if target.session != nil {
			return opforengine.Bool(!target.session.IsDead), nil
		}
		return opforengine.Bool(target.pulse != nil && !target.pulse.IsDead), nil
	case opforengine.AggressorSessionQueryIsAdmin:
		integrity := strings.ToLower(targetIntegrity(target))
		return opforengine.Bool(strings.Contains(integrity, "high") || strings.Contains(integrity, "system") || strings.Contains(integrity, "admin")), nil
	case opforengine.AggressorSessionQueryIsBeacon:
		return opforengine.Bool(target.pulse != nil), nil
	case opforengine.AggressorSessionQueryIsSSH:
		return opforengine.Bool(false), nil
	default:
		return opforengine.Null(), fmt.Errorf("opfor: unsupported session status query %q", query.Name)
	}
}

func targetIntegrity(target resolvedTarget) string {
	if target.session != nil {
		return target.session.Integrity
	}
	if target.pulse != nil {
		return target.pulse.Integrity
	}
	return ""
}

func (manager *Manager) allTargets(ctx context.Context) ([]resolvedTarget, error) {
	rpc, err := manager.rpc()
	if err != nil {
		return nil, err
	}
	sessions, err := rpc.GetSessions(ctx, &commonpb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("opfor: list sessions: %w", err)
	}
	pulses, err := rpc.GetPulses(ctx, &commonpb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("opfor: list pulses: %w", err)
	}
	targets := make([]resolvedTarget, 0, len(sessions.GetSessions())+len(pulses.GetPulses()))
	for _, session := range sessions.GetSessions() {
		if session != nil {
			targets = append(targets, resolvedTarget{session: session})
		}
	}
	for _, pulse := range pulses.GetPulses() {
		if pulse != nil {
			targets = append(targets, resolvedTarget{pulse: pulse})
		}
	}
	return targets, nil
}

func pulseArchitecture(arch string) (string, error) {
	switch strings.ToLower(arch) {
	case "amd64", "x64", "x86_64":
		return "x64", nil
	case "386", "x86", "i386":
		return "x86", nil
	default:
		return "", fmt.Errorf("opfor: unsupported BOF target architecture %q", arch)
	}
}

func targetMetadata(target resolvedTarget) *opforengine.Hash {
	metadata := opforengine.NewOrderedHash()
	set := func(key string, value opforengine.Value) { metadata.Set(key, value) }
	if target.session != nil {
		appendTargetMetadata(set, target.session.ID, target.session.Name, target.session.Hostname,
			target.session.Username, target.session.OS, target.session.Arch, target.session.Transport,
			target.session.RemoteAddress, target.session.Filename, target.session.Version,
			target.session.Integrity, target.session.PID, !target.session.IsDead, false)
	} else if target.pulse != nil {
		appendTargetMetadata(set, target.pulse.ID, target.pulse.Name, target.pulse.Hostname,
			target.pulse.Username, target.pulse.OS, target.pulse.Arch, target.pulse.Transport,
			target.pulse.RemoteAddress, target.pulse.Filename, target.pulse.Version,
			target.pulse.Integrity, target.pulse.PID, !target.pulse.IsDead, true)
	}
	return metadata
}

func appendTargetMetadata(
	set func(string, opforengine.Value),
	id, name, hostname, username, osName, arch, transport, remoteAddress, filename, version, integrity string,
	pid int32,
	active, pulse bool,
) {
	architecture, err := pulseArchitecture(arch)
	if err != nil {
		architecture = strings.ToLower(arch)
	}
	set("id", opforengine.String(id))
	set("name", opforengine.String(name))
	set("computer", opforengine.String(hostname))
	set("host", opforengine.String(hostname))
	set("user", opforengine.String(username))
	set("os", opforengine.String(osName))
	set("arch", opforengine.String(architecture))
	set("transport", opforengine.String(transport))
	set("external", opforengine.String(remoteAddress))
	set("process", opforengine.String(filename))
	set("version", opforengine.String(version))
	set("integrity", opforengine.String(integrity))
	set("pid", opforengine.Int(pid))
	set("active", opforengine.Bool(active))
	set("pulse", opforengine.Bool(pulse))
}
