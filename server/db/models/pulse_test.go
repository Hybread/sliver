package models

import "testing"

func TestPulseToProtobufCapabilities(t *testing.T) {
	pulse := &Pulse{Capabilities: 42}

	pb := pulse.ToProtobuf()
	if pb.Capabilities != pulse.Capabilities {
		t.Fatalf("expected Capabilities=%d, got %d", pulse.Capabilities, pb.Capabilities)
	}
}
