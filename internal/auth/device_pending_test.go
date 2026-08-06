package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/securestore"
)

func TestPendingDeviceLoginDoesNotPersistRawDeviceCode(t *testing.T) {
	store := securestore.NewMemory()
	deviceCode := "vcm_dc_must_not_be_persisted"
	pending := PendingDeviceLogin{
		SchemaVersion: 1, ProfileID: "default", ProfileName: "default", Region: "cn",
		APIBaseURL: "https://api.viceme.cn", APIOrigin: "https://api.viceme.cn",
		IntervalSeconds: 5, ExpiresAt: time.Now().Add(time.Minute),
	}
	if err := SavePendingDeviceLogin(store, deviceCode, pending); err != nil {
		t.Fatal(err)
	}
	value, err := store.Get(PendingDeviceLoginKey(deviceCode))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(value, deviceCode) || strings.Contains(PendingDeviceLoginKey(deviceCode), deviceCode) {
		t.Fatal("raw device code was persisted")
	}
	loaded, err := LoadPendingDeviceLogin(store, deviceCode)
	if err != nil || loaded.ProfileID != pending.ProfileID {
		t.Fatalf("pending metadata did not round-trip: %#v err=%v", loaded, err)
	}
}
