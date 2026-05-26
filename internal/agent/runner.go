package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/example/staccato/internal/manifest"
	"github.com/example/staccato/internal/queue"
)

type Runner struct {
	manifest  manifest.Manifest
	bus       *queue.Bus
	objectDir string
	metrics   queue.ConsumptionMetrics
}

func NewRunner(mf manifest.Manifest, bus *queue.Bus, objectDir string) *Runner {
	return &Runner{manifest: mf, bus: bus, objectDir: objectDir}
}

func (r *Runner) Run(ctx context.Context) error {
	if err := r.register(); err != nil {
		return err
	}
	if _, err := r.bus.SubscribeJSON(queue.SubjectCommand(r.manifest.AgentID()), func(data []byte) {
		var req queue.CommandRequest
		if json.Unmarshal(data, &req) == nil {
			go r.handleCommand(ctx, req)
		}
	}); err != nil {
		return err
	}
	if _, err := r.bus.SubscribeJSON(queue.SubjectFileRequest(r.manifest.AgentID()), func(data []byte) {
		var req queue.FileRequest
		if json.Unmarshal(data, &req) == nil {
			go r.handleFileRequest(req)
		}
	}); err != nil {
		return err
	}
	if _, err := r.bus.SubscribeJSON(queue.SubjectLogRequest(r.manifest.AgentID()), func(data []byte) {
		var req queue.LogRequest
		if json.Unmarshal(data, &req) == nil {
			go r.handleLogRequest(ctx, req)
		}
	}); err != nil {
		return err
	}
	if _, err := r.bus.SubscribeJSON(queue.SubjectCapabilityRequest(r.manifest.AgentID()), func(data []byte) {
		var req queue.CapabilityRequest
		if json.Unmarshal(data, &req) == nil {
			go r.handleCapabilityRequest(req)
		}
	}); err != nil {
		return err
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_ = r.heartbeat()
		}
	}
}

func (r *Runner) register() error {
	reg := queue.RegisterAgent{
		AgentID:      r.manifest.AgentID(),
		Name:         r.manifest.Name,
		Repo:         r.manifest.Repo,
		Scripts:      r.scriptCapabilities(),
		Files:        r.fileCapabilities(),
		RegisteredAt: time.Now().UTC(),
	}
	return r.bus.PublishJSON(queue.SubjectAgentRegister, reg)
}

func (r *Runner) heartbeat() error {
	hb := queue.Heartbeat{
		AgentID:      r.manifest.AgentID(),
		SentAt:       time.Now().UTC(),
		Environments: r.discoverEnvironments(),
		Consumption:  r.consumption(),
	}
	return r.bus.PublishJSON(queue.SubjectAgentHeartbeat, hb)
}

func (r *Runner) handleCommand(ctx context.Context, req queue.CommandRequest) {
	atomic.AddUint64(&r.metrics.CommandsStarted, 1)
	r.emit(req, "started", "", "command started", 0)

	script, ok := r.resolveScript(req.Scope, req.Name)
	if !ok {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", "command is not allowed by manifest", 1)
		return
	}

	timeout := 2 * time.Minute
	if script.Timeout != "" {
		parsed, err := time.ParseDuration(script.Timeout)
		if err != nil {
			atomic.AddUint64(&r.metrics.CommandsFailed, 1)
			r.emit(req, "failed", "", err.Error(), 1)
			return
		}
		timeout = parsed
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	scriptPath, err := filepath.Abs(r.manifest.Resolve(script.Path))
	if err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", err.Error(), 1)
		return
	}

	args := append([]string{}, req.Args...)
	cmd := exec.CommandContext(ctx, scriptPath, args...)
	cmd.Env = append(os.Environ(),
		"STACCATO_AGENT_ID="+r.manifest.AgentID(),
		"STACCATO_REPO="+strings.TrimSpace(r.manifest.Repo),
	)
	if req.Scope == "env" && req.Environment != "" {
		cmd.Dir = r.manifest.Resolve(filepath.Join(r.manifest.Environments, req.Environment))
		cmd.Env = append(cmd.Env, "STACCATO_ENVIRONMENT="+req.Environment)
	}

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", err.Error(), 1)
		return
	}
	go r.scan(req, "stdout", stdout)
	go r.scan(req, "stderr", stderr)

	if err := cmd.Wait(); err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", err.Error(), cmd.ProcessState.ExitCode())
		return
	}
	atomic.AddUint64(&r.metrics.CommandsSucceeded, 1)
	r.emit(req, "succeeded", "", "command succeeded", 0)
	// Refresh environment status immediately so UI reflects state-changing commands quickly.
	_ = r.heartbeat()
}

func (r *Runner) scan(req queue.CommandRequest, stream string, pipe interface{ Read([]byte) (int, error) }) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		r.emit(req, "output", stream, scanner.Text(), 0)
	}
}

func (r *Runner) emit(req queue.CommandRequest, status, stream, message string, exitCode int) {
	_ = r.bus.PublishJSON(queue.SubjectCommandEvent(req.CommandID), queue.CommandEvent{
		CommandID:   req.CommandID,
		AgentID:     req.AgentID,
		Scope:       req.Scope,
		Name:        req.Name,
		Environment: req.Environment,
		Args:        append([]string{}, req.Args...),
		Status:      status,
		Stream:      stream,
		Message:     message,
		ExitCode:    exitCode,
		SentAt:      time.Now().UTC(),
	})
}

func (r *Runner) resolveScript(scope, name string) (manifest.Script, bool) {
	switch scope {
	case "vm":
		script, ok := r.manifest.VMScripts[name]
		return script, ok
	case "env":
		script, ok := r.manifest.EnvScripts[name]
		return script, ok
	default:
		return manifest.Script{}, false
	}
}

func (r *Runner) discoverEnvironments() []queue.EnvironmentStatus {
	entries, err := os.ReadDir(r.manifest.Resolve(r.manifest.Environments))
	if err != nil {
		return nil
	}
	composeFile := r.manifest.Docker.ComposeFile
	if composeFile == "" {
		composeFile = "docker-compose.yaml"
	}
	var envs []queue.EnvironmentStatus
	for _, entry := range entries {
		if entry.IsDir() {
			envName := entry.Name()
			envDir := r.manifest.Resolve(filepath.Join(r.manifest.Environments, envName))
			branch, commit, detached := discoverGitHead(envDir)
			envs = append(envs, queue.EnvironmentStatus{
				Name:     envName,
				Branch:   branch,
				Commit:   commit,
				Detached: detached,
				Services: r.discoverComposeServices(envDir, composeFile),
			})
		}
	}
	return envs
}

func discoverGitHead(envDir string) (branch, commit string, detached bool) {
	branchCmd := exec.Command("git", "branch", "--show-current")
	branchCmd.Dir = envDir
	branchOut, branchErr := branchCmd.Output()
	if branchErr == nil {
		branch = strings.TrimSpace(string(branchOut))
	}

	commitCmd := exec.Command("git", "rev-parse", "HEAD")
	commitCmd.Dir = envDir
	commitOut, commitErr := commitCmd.Output()
	if commitErr == nil {
		commit = strings.TrimSpace(string(commitOut))
	}

	if commit == "" {
		return "", "", false
	}
	if branch == "" {
		return "", commit, true
	}
	return branch, commit, false
}

func (r *Runner) discoverComposeServices(envDir, composeFile string) map[string]string {
	args := []string{"compose", "-f", composeFile, "ps", "--format", "json"}
	cmd := exec.Command("docker", args...)
	cmd.Dir = envDir
	output, err := cmd.Output()
	if err != nil {
		return map[string]string{"docker-compose": "unavailable"}
	}

	services := parseComposePSOutput(output)
	if len(services) == 0 {
		return map[string]string{"docker-compose": "empty"}
	}
	return services
}

type composePSRow struct {
	Name   string `json:"Name"`
	Service string `json:"Service"`
	State  string `json:"State"`
	Status string `json:"Status"`
}

func parseComposePSOutput(output []byte) map[string]string {
	result := make(map[string]string)

	var rows []composePSRow
	if err := json.Unmarshal(output, &rows); err == nil {
		for _, row := range rows {
			name := strings.TrimSpace(row.Service)
			if name == "" {
				name = strings.TrimSpace(row.Name)
			}
			if name == "" {
				continue
			}
			status := strings.TrimSpace(row.Status)
			if status == "" {
				status = strings.TrimSpace(row.State)
			}
			if status == "" {
				status = "unknown"
			}
			result[name] = status
		}
		return result
	}

	// Some docker versions emit one JSON object per line.
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row composePSRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		name := strings.TrimSpace(row.Service)
		if name == "" {
			name = strings.TrimSpace(row.Name)
		}
		if name == "" {
			continue
		}
		status := strings.TrimSpace(row.Status)
		if status == "" {
			status = strings.TrimSpace(row.State)
		}
		if status == "" {
			status = "unknown"
		}
		result[name] = status
	}

	return result
}

func (r *Runner) handleFileRequest(req queue.FileRequest) {
	path, ok := r.manifest.Files[req.FileKey]
	if !ok {
		r.publishFileResponse(req, "", "", 0, "file is not allowed by manifest")
		return
	}
	fileName := filepath.Base(path)

	source := r.manifest.Resolve(path)
	input, err := os.Open(source)
	if err != nil {
		r.publishFileResponse(req, "", fileName, 0, err.Error())
		return
	}
	defer input.Close()

	if err := os.MkdirAll(r.objectDir, 0o755); err != nil {
		r.publishFileResponse(req, "", fileName, 0, err.Error())
		return
	}
	objectName := fmt.Sprintf("%s-%s", req.RequestID, filepath.Base(source))
	destination := filepath.Join(r.objectDir, objectName)
	output, err := os.Create(destination)
	if err != nil {
		r.publishFileResponse(req, "", fileName, 0, err.Error())
		return
	}
	size, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		r.publishFileResponse(req, "", fileName, 0, copyErr.Error())
		return
	}
	if closeErr != nil {
		r.publishFileResponse(req, "", fileName, 0, closeErr.Error())
		return
	}

	atomic.AddUint64(&r.metrics.BytesUploaded, uint64(size))
	r.publishFileResponse(req, "file://"+destination, fileName, size, "")
}

func (r *Runner) handleLogRequest(ctx context.Context, req queue.LogRequest) {
	if req.Tail <= 0 {
		req.Tail = 100
	}
	envDir := r.manifest.Resolve(filepath.Join(r.manifest.Environments, req.Environment))
	composeFile := r.manifest.Docker.ComposeFile
	if composeFile == "" {
		composeFile = "docker-compose.yaml"
	}
	args := []string{"compose", "-f", composeFile, "logs", "--tail", fmt.Sprintf("%d", req.Tail)}
	selectedService := strings.TrimSpace(req.Service)
	if selectedService != "" {
		selectedService = r.resolveComposeServiceName(envDir, composeFile, selectedService)
		args = append(args, selectedService)
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = envDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if selectedService != "" {
			fallbackArgs := []string{"compose", "-f", composeFile, "logs", "--tail", fmt.Sprintf("%d", req.Tail)}
			fallbackCmd := exec.CommandContext(ctx, "docker", fallbackArgs...)
			fallbackCmd.Dir = envDir
			fallbackOutput, fallbackErr := fallbackCmd.CombinedOutput()
			if fallbackErr == nil {
				r.publishLogLine(req, fmt.Sprintf("docker compose logs for service %q unavailable; showing all services", selectedService))
				r.streamLogOutput(req, string(fallbackOutput))
				return
			}
		}
		r.publishLogLine(req, fmt.Sprintf("docker compose logs unavailable: %v", err))
		return
	}
	r.streamLogOutput(req, string(output))
}

func (r *Runner) streamLogOutput(req queue.LogRequest, output string) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		atomic.AddUint64(&r.metrics.LogLinesStreamed, 1)
		r.publishLogLine(req, scanner.Text())
	}
}

func (r *Runner) resolveComposeServiceName(envDir, composeFile, target string) string {
	args := []string{"compose", "-f", composeFile, "ps", "--format", "json"}
	cmd := exec.Command("docker", args...)
	cmd.Dir = envDir
	output, err := cmd.Output()
	if err != nil {
		return target
	}

	trimmedTarget := strings.TrimSpace(target)
	if trimmedTarget == "" {
		return target
	}

	var rows []composePSRow
	if err := json.Unmarshal(output, &rows); err == nil {
		for _, row := range rows {
			if strings.TrimSpace(row.Name) == trimmedTarget {
				service := strings.TrimSpace(row.Service)
				if service != "" {
					return service
				}
			}
		}
		return target
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row composePSRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		if strings.TrimSpace(row.Name) == trimmedTarget {
			service := strings.TrimSpace(row.Service)
			if service != "" {
				return service
			}
		}
	}

	return target
}

func (r *Runner) publishLogLine(req queue.LogRequest, line string) {
	_ = r.bus.PublishJSON(queue.SubjectLogs(req.AgentID, req.Environment), queue.LogEvent{
		AgentID:     req.AgentID,
		Environment: req.Environment,
		Line:        line,
		SentAt:      time.Now().UTC(),
	})
}

func (r *Runner) publishFileResponse(req queue.FileRequest, objectURL, fileName string, size int64, message string) {
	_ = r.bus.PublishJSON(queue.SubjectFileResponse(req.RequestID), queue.FileResponse{
		RequestID: req.RequestID,
		AgentID:   req.AgentID,
		FileKey:   req.FileKey,
		FileName:  fileName,
		ObjectURL: objectURL,
		Size:      size,
		Error:     message,
		SentAt:    time.Now().UTC(),
	})
}

func (r *Runner) handleCapabilityRequest(_ queue.CapabilityRequest) {
	_ = r.register()
}

func (r *Runner) consumption() queue.ConsumptionMetrics {
	return queue.ConsumptionMetrics{
		CommandsStarted:   atomic.LoadUint64(&r.metrics.CommandsStarted),
		CommandsSucceeded: atomic.LoadUint64(&r.metrics.CommandsSucceeded),
		CommandsFailed:    atomic.LoadUint64(&r.metrics.CommandsFailed),
		BytesUploaded:     atomic.LoadUint64(&r.metrics.BytesUploaded),
		LogLinesStreamed:  atomic.LoadUint64(&r.metrics.LogLinesStreamed),
	}
}

func (r *Runner) scriptCapabilities() []queue.ScriptCapability {
	var caps []queue.ScriptCapability
	for name, script := range r.manifest.VMScripts {
		caps = append(caps, queue.ScriptCapability{Name: name, Scope: "vm", Args: script.Args, Timeout: script.Timeout})
	}
	for name, script := range r.manifest.EnvScripts {
		caps = append(caps, queue.ScriptCapability{Name: name, Scope: "env", Args: script.Args, Timeout: script.Timeout})
	}
	return caps
}

func (r *Runner) fileCapabilities() []queue.FileCapability {
	var caps []queue.FileCapability
	for key := range r.manifest.Files {
		caps = append(caps, queue.FileCapability{Key: key})
	}
	return caps
}

func FileObjectURL(requestID, key string) string {
	return fmt.Sprintf("object://staccato/%s/%s", requestID, key)
}
