package platform

import (
	"testing"
	"time"

	"github.com/example/envoy/internal/queue"
)

func TestMemoryStateProjectsAgentFromEvents(t *testing.T) {
	now := time.Date(2026, 5, 25, 13, 0, 0, 0, time.UTC)
	state := NewMemoryState(func() time.Time { return now })

	state.ApplyRegistration(queue.RegisterAgent{
		AgentID: "agent-1",
		Name:    "Agent One",
	})
	state.ApplyHeartbeat(queue.Heartbeat{
		AgentID: "agent-1",
		Consumption: queue.ConsumptionMetrics{
			CommandsStarted: 2,
		},
	})

	agent, ok := state.Agent("agent-1")
	if !ok {
		t.Fatal("expected agent projection")
	}
	if agent.Registration.Name != "Agent One" {
		t.Fatalf("agent name = %q", agent.Registration.Name)
	}
	if agent.Heartbeat.Consumption.CommandsStarted != 2 {
		t.Fatalf("commands started = %d", agent.Heartbeat.Consumption.CommandsStarted)
	}
	if len(state.Agents()) != 1 {
		t.Fatalf("agent count = %d", len(state.Agents()))
	}
}

func TestMemoryStateKeepsCommandEventsInMemory(t *testing.T) {
	state := NewMemoryState(time.Now)
	state.ApplyCommandEvent(queue.CommandEvent{CommandID: "cmd-1", Status: "started"})
	state.ApplyCommandEvent(queue.CommandEvent{CommandID: "cmd-1", Status: "succeeded"})

	events := state.CommandEvents("cmd-1")
	if len(events) != 2 {
		t.Fatalf("event count = %d", len(events))
	}
	if events[1].Status != "succeeded" {
		t.Fatalf("last status = %q", events[1].Status)
	}
}

func TestMemoryStateKeepsFileResponsesInMemory(t *testing.T) {
	state := NewMemoryState(time.Now)
	state.ApplyFileResponse(queue.FileResponse{
		RequestID: "file-1",
		FileKey:   "sample",
		ObjectURL: "file://var/objects/file-1-sample.txt",
		Size:      12,
	})

	files := state.FileResponses()
	if len(files) != 1 {
		t.Fatalf("file response count = %d", len(files))
	}
	if files[0].FileKey != "sample" {
		t.Fatalf("file key = %q", files[0].FileKey)
	}
}
