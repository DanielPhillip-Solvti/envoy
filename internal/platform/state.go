package platform

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/example/envoy/internal/queue"
)

type State struct {
	mu     sync.RWMutex
	now    func() time.Time
	agents map[string]AgentView
	events map[string][]queue.CommandEvent
	files  map[string]queue.FileResponse
	logs   map[string][]queue.LogEvent
}

type AgentView struct {
	Registration queue.RegisterAgent
	Heartbeat    queue.Heartbeat
	LastSeenAt   time.Time
}

func NewMemoryState(now func() time.Time) *State {
	return &State{
		now:    now,
		agents: map[string]AgentView{},
		events: map[string][]queue.CommandEvent{},
		files:  map[string]queue.FileResponse{},
		logs:   map[string][]queue.LogEvent{},
	}
}

func (s *State) ApplyRegistration(reg queue.RegisterAgent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	view := s.agents[reg.AgentID]
	view.Registration = reg
	view.LastSeenAt = s.now()
	s.agents[reg.AgentID] = view
}

func (s *State) ApplyHeartbeat(hb queue.Heartbeat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	view := s.agents[hb.AgentID]
	view.Heartbeat = hb
	view.LastSeenAt = s.now()
	s.agents[hb.AgentID] = view
}

func (s *State) ApplyCommandEvent(event queue.CommandEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[event.CommandID] = append(s.events[event.CommandID], event)
}

func (s *State) ApplyFileResponse(response queue.FileResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[response.RequestID] = response
}

func (s *State) ApplyLogEvent(event queue.LogEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := event.AgentID + "/" + event.Environment
	s.logs[key] = append(s.logs[key], event)
}

func (s *State) Agents() []AgentView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]AgentView, 0, len(s.agents))
	for _, agent := range s.agents {
		result = append(result, agent)
	}
	return result
}

func (s *State) Agent(agentID string) (AgentView, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	agent, ok := s.agents[agentID]
	return agent, ok
}

func (s *State) CommandEvents(commandID string) []queue.CommandEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]queue.CommandEvent, len(s.events[commandID]))
	copy(result, s.events[commandID])
	return result
}

func (s *State) FileResponses() []queue.FileResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]queue.FileResponse, 0, len(s.files))
	for _, response := range s.files {
		result = append(result, response)
	}
	return result
}

func (s *State) Logs(agentID, environment string) []queue.LogEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := agentID + "/" + environment
	result := make([]queue.LogEvent, len(s.logs[key]))
	copy(result, s.logs[key])
	return result
}

func Subscribe(_ context.Context, bus *queue.Bus, state *State) error {
	if _, err := bus.SubscribeJSON(queue.SubjectAgentRegister, func(data []byte) {
		var reg queue.RegisterAgent
		if json.Unmarshal(data, &reg) == nil {
			state.ApplyRegistration(reg)
		}
	}); err != nil {
		return err
	}
	if _, err := bus.SubscribeJSON(queue.SubjectAgentHeartbeat, func(data []byte) {
		var hb queue.Heartbeat
		if json.Unmarshal(data, &hb) == nil {
			state.ApplyHeartbeat(hb)
		}
	}); err != nil {
		return err
	}
	if _, err := bus.SubscribeJSON("envoy.command.event.*", func(data []byte) {
		var event queue.CommandEvent
		if json.Unmarshal(data, &event) == nil {
			state.ApplyCommandEvent(event)
		}
	}); err != nil {
		return err
	}
	if _, err := bus.SubscribeJSON("envoy.file.response.*", func(data []byte) {
		var response queue.FileResponse
		if json.Unmarshal(data, &response) == nil {
			state.ApplyFileResponse(response)
		}
	}); err != nil {
		return err
	}
	if _, err := bus.SubscribeJSON("envoy.logs.*.*", func(data []byte) {
		var event queue.LogEvent
		if json.Unmarshal(data, &event) == nil {
			state.ApplyLogEvent(event)
		}
	}); err != nil {
		return err
	}
	return nil
}
