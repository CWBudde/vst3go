package plugin

import (
	"testing"
)

func TestUIDGeneration(t *testing.T) {
	tests := []struct {
		name     string
		pluginID string
	}{
		{
			name:     "Plugin generates deterministic UID",
			pluginID: "com.mycompany.newplugin",
		},
		{
			name:     "Different plugin also generates deterministic UID",
			pluginID: "com.mycompany.anotherplugin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &Info{ID: tt.pluginID}

			// Generate UID twice
			uid1 := info.UID()
			uid2 := info.UID()

			// Should always be deterministic
			if uid1 != uid2 {
				t.Errorf("UID generation is not deterministic for %s", tt.pluginID)
			}

			// Validate UID
			if err := info.ValidateUID(); err != nil {
				t.Errorf("UID validation failed for %s: %v", tt.pluginID, err)
			}
		})
	}
}

func TestUIDUniqueness(t *testing.T) {
	// Test that different plugin IDs generate different UIDs
	plugins := []string{
		"com.company1.plugin1",
		"com.company1.plugin2",
		"com.company2.plugin1",
		"com.different.name",
	}

	uids := make(map[[16]byte]string)

	for _, pluginID := range plugins {
		info := &Info{ID: pluginID}
		uid := info.UID()

		if existingID, exists := uids[uid]; exists {
			t.Errorf("UID collision between %s and %s", pluginID, existingID)
		}

		uids[uid] = pluginID
	}
}

func TestUIDValidation(t *testing.T) {
	tests := []struct {
		name    string
		info    Info
		wantErr bool
	}{
		{
			name:    "Valid plugin ID",
			info:    Info{ID: "com.example.plugin"},
			wantErr: false,
		},
		{
			name:    "Empty plugin ID",
			info:    Info{ID: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.info.ValidateUID()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
