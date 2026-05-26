package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/example/staccato/internal/platform"
	"github.com/example/staccato/internal/queue"
	"github.com/nats-io/nkeys"
)

type natsEnv struct {
	url              string
	platformSeedPath string
	agentSeedPath    string
	command          *exec.Cmd
}

func TestRejectTrafficWithBadNKey(t *testing.T) {
	env := startAuthenticatedNATS(t)
	t.Cleanup(func() { stopNATS(t, env) })

	badSeed, _, err := createUserNKey()
	if err != nil {
		t.Fatalf("create bad nkey: %v", err)
	}
	badSeedPath := filepath.Join(t.TempDir(), "bad.nk")
	if err := os.WriteFile(badSeedPath, []byte(badSeed+"\n"), 0o600); err != nil {
		t.Fatalf("write bad nkey seed: %v", err)
	}

	if _, err := queue.Connect(env.url, badSeedPath); err == nil {
		t.Fatal("expected bad nkey connection to be rejected")
	}
}

func TestIgnoreEventsFromUnconfirmedAgentUntilActivation(t *testing.T) {
	env := startAuthenticatedNATS(t)
	t.Cleanup(func() { stopNATS(t, env) })

	platformBus, err := queue.Connect(env.url, env.platformSeedPath)
	if err != nil {
		t.Fatalf("connect platform bus: %v", err)
	}
	defer platformBus.Close()

	agentBus, err := queue.Connect(env.url, env.agentSeedPath)
	if err != nil {
		t.Fatalf("connect agent bus: %v", err)
	}
	defer agentBus.Close()

	state := platform.NewMemoryState(time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := platform.Subscribe(ctx, platformBus, state); err != nil {
		t.Fatalf("subscribe platform state: %v", err)
	}

	const agentID = "agent-e2e"
	if err := agentBus.PublishJSON(queue.SubjectAgentRegister, queue.RegisterAgent{
		AgentID:      agentID,
		Name:         "E2E Agent",
		RegisteredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("publish register: %v", err)
	}

	if err := waitFor(2*time.Second, func() bool {
		_, ok := state.Agent(agentID)
		return ok
	}); err != nil {
		t.Fatalf("agent registration did not reach platform state: %v", err)
	}

	if err := agentBus.PublishJSON(queue.SubjectCommandEvent("cmd-unconfirmed"), queue.CommandEvent{
		CommandID: "cmd-unconfirmed",
		AgentID:   agentID,
		Status:    "started",
		SentAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("publish command event before activation: %v", err)
	}

	// Give the subscriber enough time to process any in-flight messages.
	time.Sleep(150 * time.Millisecond)
	if got := len(state.CommandEvents("cmd-unconfirmed")); got != 0 {
		t.Fatalf("unconfirmed agent event count = %d, want 0", got)
	}

	state.ActivateAgent(agentID)
	if err := agentBus.PublishJSON(queue.SubjectCommandEvent("cmd-confirmed"), queue.CommandEvent{
		CommandID: "cmd-confirmed",
		AgentID:   agentID,
		Status:    "started",
		SentAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("publish command event after activation: %v", err)
	}

	if err := waitFor(2*time.Second, func() bool {
		return len(state.CommandEvents("cmd-confirmed")) == 1
	}); err != nil {
		t.Fatalf("confirmed agent event did not reach platform state: %v", err)
	}
}

func startAuthenticatedNATS(t *testing.T) *natsEnv {
	t.Helper()

	natsServerPath, err := findNATSServerPath()
	if err != nil {
		t.Skip("nats-server not found; set NATS_SERVER or install in PATH or ~/go/bin")
	}

	platformSeed, platformPub, err := createUserNKey()
	if err != nil {
		t.Fatalf("create platform nkey: %v", err)
	}
	agentSeed, agentPub, err := createUserNKey()
	if err != nil {
		t.Fatalf("create agent nkey: %v", err)
	}

	tmp := t.TempDir()
	platformSeedPath := filepath.Join(tmp, "platform.nk")
	agentSeedPath := filepath.Join(tmp, "agent.nk")
	if err := os.WriteFile(platformSeedPath, []byte(platformSeed+"\n"), 0o600); err != nil {
		t.Fatalf("write platform seed: %v", err)
	}
	if err := os.WriteFile(agentSeedPath, []byte(agentSeed+"\n"), 0o600); err != nil {
		t.Fatalf("write agent seed: %v", err)
	}

	clientPort := freeTCPPort(t)
	monitorPort := freeTCPPort(t)
	configPath := filepath.Join(tmp, "server.conf")
	cfg := natsConfig(clientPort, monitorPort, platformPub, agentPub)
	if err := os.WriteFile(configPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write nats config: %v", err)
	}

	cmd := exec.Command(natsServerPath, "-c", configPath)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start nats-server: %v", err)
	}

	env := &natsEnv{
		url:              fmt.Sprintf("nats://127.0.0.1:%d", clientPort),
		platformSeedPath: platformSeedPath,
		agentSeedPath:    agentSeedPath,
		command:          cmd,
	}

	if err := waitFor(3*time.Second, func() bool {
		bus, err := queue.Connect(env.url, env.platformSeedPath)
		if err != nil {
			return false
		}
		bus.Close()
		return true
	}); err != nil {
		stopNATS(t, env)
		t.Fatalf("nats-server did not become ready: %v", err)
	}

	return env
}

func findNATSServerPath() (string, error) {
	if configured := os.Getenv("NATS_SERVER"); configured != "" {
		if _, err := os.Stat(configured); err == nil {
			return configured, nil
		}
	}

	path, err := exec.LookPath("nats-server")
	if err == nil {
		return path, nil
	}

	home, homeErr := os.UserHomeDir()
	if homeErr == nil {
		candidate := filepath.Join(home, "go", "bin", "nats-server")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}

	return "", err
}

func stopNATS(t *testing.T, env *natsEnv) {
	t.Helper()
	if env == nil || env.command == nil || env.command.Process == nil {
		return
	}
	_ = env.command.Process.Kill()
	_, _ = env.command.Process.Wait()
}

func createUserNKey() (seed string, public string, err error) {
	kp, err := nkeys.CreateUser()
	if err != nil {
		return "", "", err
	}
	seedBytes, err := kp.Seed()
	if err != nil {
		return "", "", err
	}
	publicBytes, err := kp.PublicKey()
	if err != nil {
		return "", "", err
	}
	return string(seedBytes), string(publicBytes), nil
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate free tcp port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitFor(timeout time.Duration, condition func() bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("condition not met within %s", timeout)
}

func natsConfig(clientPort, monitorPort int, platformPub, agentPub string) string {
	return fmt.Sprintf(`port: %d
http_port: %d
jetstream: enabled

authorization {
  users: [
    {
      nkey: %q
      permissions: {
        publish: [
          "staccato.command.*"
          "staccato.file.request.*"
          "staccato.log.request.*"
          "staccato.capability.request.*"
        ]
        subscribe: [
          "staccato.agent.register"
          "staccato.agent.heartbeat"
          "staccato.command.event.*"
          "staccato.file.response.*"
          "staccato.logs.*.*"
        ]
      }
    }
    {
      nkey: %q
      permissions: {
        publish: [
          "staccato.agent.register"
          "staccato.agent.heartbeat"
          "staccato.command.event.*"
          "staccato.file.response.*"
          "staccato.logs.*.*"
        ]
        subscribe: [
          "staccato.command.*"
          "staccato.file.request.*"
          "staccato.log.request.*"
          "staccato.capability.request.*"
        ]
      }
    }
  ]
}
`, clientPort, monitorPort, platformPub, agentPub)
}
