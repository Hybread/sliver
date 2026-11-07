package handlers

import (
	"path/filepath"
	"testing"
	uuid "uuid"

	"github.com/bishopfox/sliver/protobuf/sliverpb"
	"github.com/bishopfox/sliver/server/core"
	"github.com/bishopfox/sliver/server/db"
	"github.com/bishopfox/sliver/server/db/models"
	"google.golang.org/protobuf/proto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPulseRegisterHandlerPropagatesCapabilities(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pulse-capabilities.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := testDB.AutoMigrate(&models.Pulse{}, &models.PulseTask{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	originalDB := db.Client
	db.Client = testDB
	t.Cleanup(func() {
		db.Client = originalDB
		sqlDB, dbErr := testDB.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	pulseID := uuid.NewV4()
	registerData, err := proto.Marshal(&sliverpb.PulseRegister{
		ID: pulseID.String(),
		Register: &sliverpb.Register{
			Uuid:         pulseID.String(),
			Capabilities: sliverpb.CapabilityBOFV1,
		},
	})
	if err != nil {
		t.Fatalf("marshal pulse register: %v", err)
	}

	pulseRegisterHandler(core.NewImplantConnection("test", "n/a"), registerData)

	pulse, err := db.PulseByID(pulseID.String())
	if err != nil {
		t.Fatalf("load registered pulse: %v", err)
	}
	if pulse.Capabilities != sliverpb.CapabilityBOFV1 {
		t.Fatalf("expected Capabilities=%d, got %d", sliverpb.CapabilityBOFV1, pulse.Capabilities)
	}
	if pulse.ToProtobuf().Capabilities != sliverpb.CapabilityBOFV1 {
		t.Fatalf("expected protobuf Capabilities=%d, got %d", sliverpb.CapabilityBOFV1, pulse.ToProtobuf().Capabilities)
	}
}
