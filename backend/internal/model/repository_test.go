package model

import (
	"encoding/json"
	"testing"
)

func TestRepositoryJSONOmitsRcloneConfigAndIncludesPresenceFlag(t *testing.T) {
	payload, err := json.Marshal(Repository{
		ID:                    1,
		Name:                  "repo",
		Type:                  "rclone",
		Endpoint:              "bucket/path",
		RcloneConfig:          "legacy",
		RcloneConfigEncrypted: "ciphertext",
		HasRcloneConfig:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, exists := decoded["rclone_config"]; exists {
		t.Fatalf("expected rclone_config to be omitted, got %s", string(payload))
	}
	if decoded["has_rclone_config"] != true {
		t.Fatalf("expected has_rclone_config flag, got %s", string(payload))
	}
}
