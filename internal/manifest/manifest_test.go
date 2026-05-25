package manifest

import "testing"

func TestValidateRejectsTraversalPaths(t *testing.T) {
	mf := Manifest{
		Version:      1,
		Name:         "test",
		Environments: "envs",
		VMScripts: map[string]Script{
			"bad": {Path: "../secret.sh"},
		},
	}

	if err := mf.Validate("."); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
}

func TestValidateAcceptsDeclaredCapabilities(t *testing.T) {
	mf := Manifest{
		Version:      1,
		Name:         "Local Test VM",
		Environments: "test/vm/envs",
		VMScripts: map[string]Script{
			"health": {Path: "test/vm/scripts/health.sh"},
		},
		EnvScripts: map[string]Script{
			"restart": {Path: "test/vm/scripts/restart.sh"},
		},
		Files: map[string]string{
			"sample": "test/vm/files/sample.txt",
		},
	}

	if err := mf.Validate("."); err != nil {
		t.Fatalf("validate manifest: %v", err)
	}
	if got, want := mf.AgentID(), "local-test-vm"; got != want {
		t.Fatalf("AgentID() = %q, want %q", got, want)
	}
}
