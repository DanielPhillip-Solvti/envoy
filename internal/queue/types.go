package queue

import (
	"fmt"
	"time"
)

const (
	SubjectAgentRegister  = "staccato.agent.register"
	SubjectAgentHeartbeat = "staccato.agent.heartbeat"
)

func SubjectCommand(agentID string) string {
	return fmt.Sprintf("staccato.command.%s", agentID)
}

func SubjectCommandEvent(commandID string) string {
	return fmt.Sprintf("staccato.command.event.%s", commandID)
}

func SubjectLogs(agentID, environment string) string {
	return fmt.Sprintf("staccato.logs.%s.%s", agentID, environment)
}

func SubjectLogRequest(agentID string) string {
	return fmt.Sprintf("staccato.log.request.%s", agentID)
}

func SubjectFileRequest(agentID string) string {
	return fmt.Sprintf("staccato.file.request.%s", agentID)
}

func SubjectFileResponse(requestID string) string {
	return fmt.Sprintf("staccato.file.response.%s", requestID)
}

func SubjectCapabilityRequest(agentID string) string {
	return fmt.Sprintf("staccato.capability.request.%s", agentID)
}

func SubjectTokenUpdate(agentID string) string {
	return fmt.Sprintf("staccato.token.update.%s", agentID)
}

type ScriptCapability struct {
	Name    string   `json:"name"`
	Scope   string   `json:"scope"`
	Args    []string `json:"args"`
	Timeout string   `json:"timeout,omitempty"`
}

type FileCapability struct {
	Key string `json:"key"`
}

type EnvironmentStatus struct {
	Name     string            `json:"name"`
	Branch   string            `json:"branch,omitempty"`
	Commit   string            `json:"commit,omitempty"`
	Detached bool              `json:"detached,omitempty"`
	Services map[string]string `json:"services"`
}

type RegisterAgent struct {
	AgentID      string             `json:"agent_id"`
	Name         string             `json:"name"`
	Repo         string             `json:"repo,omitempty"`
	Scripts      []ScriptCapability `json:"scripts"`
	Files        []FileCapability   `json:"files"`
	RegisteredAt time.Time          `json:"registered_at"`
}

type Heartbeat struct {
	AgentID      string              `json:"agent_id"`
	SentAt       time.Time           `json:"sent_at"`
	Environments []EnvironmentStatus `json:"environments"`
	Consumption  ConsumptionMetrics  `json:"consumption"`
}

type ConsumptionMetrics struct {
	CommandsStarted   uint64 `json:"commands_started"`
	CommandsSucceeded uint64 `json:"commands_succeeded"`
	CommandsFailed    uint64 `json:"commands_failed"`
	BytesUploaded     uint64 `json:"bytes_uploaded"`
	LogLinesStreamed  uint64 `json:"log_lines_streamed"`
}

type CommandRequest struct {
	CommandID   string    `json:"command_id"`
	AgentID     string    `json:"agent_id"`
	Scope       string    `json:"scope"`
	Name        string    `json:"name"`
	Environment string    `json:"environment,omitempty"`
	Args        []string  `json:"args"`
	RequestedBy string    `json:"requested_by"`
	RequestedAt time.Time `json:"requested_at"`
}

type CommandEvent struct {
	CommandID   string    `json:"command_id"`
	AgentID     string    `json:"agent_id"`
	Scope       string    `json:"scope,omitempty"`
	Name        string    `json:"name,omitempty"`
	Environment string    `json:"environment,omitempty"`
	Args        []string  `json:"args,omitempty"`
	Status      string    `json:"status"`
	Stream      string    `json:"stream,omitempty"`
	Message     string    `json:"message,omitempty"`
	ExitCode    int       `json:"exit_code,omitempty"`
	SentAt      time.Time `json:"sent_at"`
}

type LogEvent struct {
	AgentID     string    `json:"agent_id"`
	Environment string    `json:"environment"`
	Line        string    `json:"line"`
	SentAt      time.Time `json:"sent_at"`
}

type LogRequest struct {
	RequestID   string    `json:"request_id"`
	AgentID     string    `json:"agent_id"`
	Environment string    `json:"environment"`
	Service     string    `json:"service,omitempty"`
	Tail        int       `json:"tail"`
	AskedAt     time.Time `json:"asked_at"`
}

type FileRequest struct {
	RequestID string    `json:"request_id"`
	AgentID   string    `json:"agent_id"`
	FileKey   string    `json:"file_key"`
	AskedAt   time.Time `json:"asked_at"`
}

type FileResponse struct {
	RequestID string    `json:"request_id"`
	AgentID   string    `json:"agent_id"`
	FileKey   string    `json:"file_key"`
	FileName  string    `json:"file_name,omitempty"`
	ObjectURL string    `json:"object_url"`
	Size      int64     `json:"size"`
	Error     string    `json:"error,omitempty"`
	SentAt    time.Time `json:"sent_at"`
}

type CapabilityRequest struct {
	AgentID     string    `json:"agent_id"`
	RequestedAt time.Time `json:"requested_at"`
}

type TokenUpdate struct {
	AgentID   string    `json:"agent_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}
