package natsauth

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nats-io/nkeys"
)

type authUser struct {
	name string
	nkey string
}

// EnsureBootstrap ensures a stable platform key exists, ensures a default agent
// key exists for local runs, and renders NATS config using all known agent keys.
func EnsureBootstrap(platformSeedPath, defaultAgentSeedPath, agentsDir, serverConfigPath string) (bool, error) {
	changed := false

	if err := os.MkdirAll(filepath.Dir(platformSeedPath), 0o755); err != nil {
		return false, fmt.Errorf("create platform seed directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(defaultAgentSeedPath), 0o755); err != nil {
		return false, fmt.Errorf("create default agent seed directory: %w", err)
	}
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return false, fmt.Errorf("create agents seed directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(serverConfigPath), 0o755); err != nil {
		return false, fmt.Errorf("create nats config directory: %w", err)
	}

	if !fileExists(platformSeedPath) {
		seed, _, err := generateUserNKey()
		if err != nil {
			return false, fmt.Errorf("generate platform nkey: %w", err)
		}
		if err := os.WriteFile(platformSeedPath, []byte(seed+"\n"), 0o600); err != nil {
			return false, fmt.Errorf("write platform seed: %w", err)
		}
		changed = true
	}

	if !fileExists(defaultAgentSeedPath) {
		seed, _, err := generateUserNKey()
		if err != nil {
			return false, fmt.Errorf("generate default agent nkey: %w", err)
		}
		if err := os.WriteFile(defaultAgentSeedPath, []byte(seed+"\n"), 0o600); err != nil {
			return false, fmt.Errorf("write default agent seed: %w", err)
		}
		changed = true
	}

	serverConfig, err := BuildServerConfig(platformSeedPath, defaultAgentSeedPath, agentsDir)
	if err != nil {
		return false, err
	}
	if current, readErr := os.ReadFile(serverConfigPath); readErr == nil && string(current) == serverConfig && !changed {
		return false, nil
	}
	if err := os.WriteFile(serverConfigPath, []byte(serverConfig), 0o644); err != nil {
		return false, fmt.Errorf("write nats config: %w", err)
	}
	return true, nil
}

// CreateAgentSeed creates a dedicated agent seed file if it does not already exist.
func CreateAgentSeed(agentID, agentsDir string) (string, bool, error) {
	cleanID := sanitizeAgentID(agentID)
	if cleanID == "" {
		return "", false, fmt.Errorf("agent id is required")
	}
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return "", false, fmt.Errorf("create agents seed directory: %w", err)
	}
	seedPath := filepath.Join(agentsDir, cleanID+".nk")
	if fileExists(seedPath) {
		return seedPath, false, nil
	}
	seed, _, err := generateUserNKey()
	if err != nil {
		return "", false, fmt.Errorf("generate agent nkey: %w", err)
	}
	if err := os.WriteFile(seedPath, []byte(seed+"\n"), 0o600); err != nil {
		return "", false, fmt.Errorf("write agent seed: %w", err)
	}
	return seedPath, true, nil
}

// BuildServerConfig renders server config from current platform and agent seed files.
func BuildServerConfig(platformSeedPath, defaultAgentSeedPath, agentsDir string) (string, error) {
	platformPublic, err := publicKeyFromSeedFile(platformSeedPath)
	if err != nil {
		return "", fmt.Errorf("read platform seed: %w", err)
	}

	agentUsersByPub := make(map[string]authUser)
	defaultAgentPublic, err := publicKeyFromSeedFile(defaultAgentSeedPath)
	if err != nil {
		return "", fmt.Errorf("read default agent seed: %w", err)
	}
	agentUsersByPub[defaultAgentPublic] = authUser{name: "default-agent", nkey: defaultAgentPublic}

	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return "", fmt.Errorf("read agents directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".nk" {
			continue
		}
		pub, err := publicKeyFromSeedFile(filepath.Join(agentsDir, entry.Name()))
		if err != nil {
			return "", fmt.Errorf("read agent seed %s: %w", entry.Name(), err)
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		agentUsersByPub[pub] = authUser{name: "agent/" + name, nkey: pub}
	}

	agentUsers := make([]authUser, 0, len(agentUsersByPub))
	for _, user := range agentUsersByPub {
		agentUsers = append(agentUsers, user)
	}
	sort.Slice(agentUsers, func(i, j int) bool {
		if agentUsers[i].name == agentUsers[j].name {
			return agentUsers[i].nkey < agentUsers[j].nkey
		}
		return agentUsers[i].name < agentUsers[j].name
	})

	return buildServerConfig(platformPublic, agentUsers), nil
}

func generateUserNKey() (seed string, public string, err error) {
	kp, err := nkeys.CreateUser()
	if err != nil {
		return "", "", err
	}
	seedBytes, err := kp.Seed()
	if err != nil {
		return "", "", err
	}
	pubBytes, err := kp.PublicKey()
	if err != nil {
		return "", "", err
	}
	return string(seedBytes), string(pubBytes), nil
}

func buildServerConfig(platformPublic string, agentUsers []authUser) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "# Generated by Staccato NATS bootstrap.\n\n")
	fmt.Fprintf(b, "port: 4222\n")
	fmt.Fprintf(b, "http_port: 8222\n")
	fmt.Fprintf(b, "jetstream: enabled\n\n")
	fmt.Fprintf(b, "authorization {\n")
	fmt.Fprintf(b, "  users: [\n")
	fmt.Fprintf(b, "    # platform\n")
	fmt.Fprintf(b, "    {\n")
	fmt.Fprintf(b, "      nkey: %q\n", platformPublic)
	fmt.Fprintf(b, "      permissions: {\n")
	fmt.Fprintf(b, "        publish: [\n")
	fmt.Fprintf(b, "          \"staccato.command.*\"\n")
	fmt.Fprintf(b, "          \"staccato.file.request.*\"\n")
	fmt.Fprintf(b, "          \"staccato.log.request.*\"\n")
	fmt.Fprintf(b, "          \"staccato.capability.request.*\"\n")
	fmt.Fprintf(b, "        ]\n")
	fmt.Fprintf(b, "        subscribe: [\n")
	fmt.Fprintf(b, "          \"staccato.agent.register\"\n")
	fmt.Fprintf(b, "          \"staccato.agent.heartbeat\"\n")
	fmt.Fprintf(b, "          \"staccato.command.event.*\"\n")
	fmt.Fprintf(b, "          \"staccato.file.response.*\"\n")
	fmt.Fprintf(b, "          \"staccato.logs.*.*\"\n")
	fmt.Fprintf(b, "        ]\n")
	fmt.Fprintf(b, "      }\n")
	fmt.Fprintf(b, "    }\n")
	for _, agentUser := range agentUsers {
		fmt.Fprintf(b, "    # %s\n", agentUser.name)
		fmt.Fprintf(b, "    {\n")
		fmt.Fprintf(b, "      nkey: %q\n", agentUser.nkey)
		fmt.Fprintf(b, "      permissions: {\n")
		fmt.Fprintf(b, "        publish: [\n")
		fmt.Fprintf(b, "          \"staccato.agent.register\"\n")
		fmt.Fprintf(b, "          \"staccato.agent.heartbeat\"\n")
		fmt.Fprintf(b, "          \"staccato.command.event.*\"\n")
		fmt.Fprintf(b, "          \"staccato.file.response.*\"\n")
		fmt.Fprintf(b, "          \"staccato.logs.*.*\"\n")
		fmt.Fprintf(b, "        ]\n")
		fmt.Fprintf(b, "        subscribe: [\n")
		fmt.Fprintf(b, "          \"staccato.command.*\"\n")
		fmt.Fprintf(b, "          \"staccato.file.request.*\"\n")
		fmt.Fprintf(b, "          \"staccato.log.request.*\"\n")
		fmt.Fprintf(b, "          \"staccato.capability.request.*\"\n")
		fmt.Fprintf(b, "        ]\n")
		fmt.Fprintf(b, "      }\n")
		fmt.Fprintf(b, "    }\n")
	}
	fmt.Fprintf(b, "  ]\n")
	fmt.Fprintf(b, "}\n")

	return b.String()
}

func publicKeyFromSeedFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	seed := strings.TrimSpace(string(data))
	kp, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		return "", err
	}
	pub, err := kp.PublicKey()
	if err != nil {
		return "", err
	}
	return pub, nil
}

func sanitizeAgentID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-")
	return replacer.Replace(value)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
