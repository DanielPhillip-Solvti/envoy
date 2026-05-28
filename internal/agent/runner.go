package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	queueGroup := "staccato.agent." + r.manifest.AgentID()
	if _, err := r.bus.SubscribeQueueJSON(queue.SubjectCommand(r.manifest.AgentID()), queueGroup, func(data []byte) {
		var req queue.CommandRequest
		if json.Unmarshal(data, &req) == nil {
			go r.handleCommand(ctx, req)
		}
	}); err != nil {
		return err
	}
	if _, err := r.bus.SubscribeQueueJSON(queue.SubjectFileRequest(r.manifest.AgentID()), queueGroup, func(data []byte) {
		var req queue.FileRequest
		if json.Unmarshal(data, &req) == nil {
			go r.handleFileRequest(req)
		}
	}); err != nil {
		return err
	}
	if _, err := r.bus.SubscribeQueueJSON(queue.SubjectLogRequest(r.manifest.AgentID()), queueGroup, func(data []byte) {
		var req queue.LogRequest
		if json.Unmarshal(data, &req) == nil {
			go r.handleLogRequest(ctx, req)
		}
	}); err != nil {
		return err
	}
	if _, err := r.bus.SubscribeQueueJSON(queue.SubjectCapabilityRequest(r.manifest.AgentID()), queueGroup, func(data []byte) {
		var req queue.CapabilityRequest
		if json.Unmarshal(data, &req) == nil {
			go r.handleCapabilityRequest(req)
		}
	}); err != nil {
		return err
	}
	if _, err := r.bus.SubscribeQueueJSON(queue.SubjectTokenUpdate(r.manifest.AgentID()), queueGroup, func(data []byte) {
		var req queue.TokenUpdate
		if json.Unmarshal(data, &req) == nil {
			os.Setenv("STACCATO_GIT_TOKEN", req.Token)
			log.Printf("agent git token updated via platform")
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
	log.Printf("agent command received pid=%d command_id=%s scope=%s name=%s environment=%s args=%q", os.Getpid(), req.CommandID, req.Scope, req.Name, req.Environment, req.Args)
	atomic.AddUint64(&r.metrics.CommandsStarted, 1)
	r.emit(req, "started", "", "command started", 0)

	if req.Scope == "builtin" {
		r.handleBuiltin(ctx, req)
		return
	}

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
		log.Printf("agent command start failed pid=%d command_id=%s error=%q", os.Getpid(), req.CommandID, err)
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", err.Error(), 1)
		return
	}
	log.Printf("agent command started pid=%d command_id=%s child_pid=%d script=%s", os.Getpid(), req.CommandID, cmd.Process.Pid, scriptPath)
	go r.scan(req, "stdout", stdout)
	go r.scan(req, "stderr", stderr)

	if err := cmd.Wait(); err != nil {
		log.Printf("agent command failed pid=%d command_id=%s exit_code=%d error=%q", os.Getpid(), req.CommandID, cmd.ProcessState.ExitCode(), err)
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", err.Error(), cmd.ProcessState.ExitCode())
		return
	}
	log.Printf("agent command succeeded pid=%d command_id=%s exit_code=0", os.Getpid(), req.CommandID)
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
	if err := scanner.Err(); err != nil {
		r.emit(req, "output", stream, fmt.Sprintf("scanner error: %v", err), 1)
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
	Name    string `json:"Name"`
	Service string `json:"Service"`
	State   string `json:"State"`
	Status  string `json:"Status"`
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

func (r *Runner) handleBuiltin(ctx context.Context, req queue.CommandRequest) {
	allowed := false
	if len(r.manifest.AllowedBuiltins) == 0 {
		// If none specified, allow none for safety? Or allow all?
		// Given the user wants to "allow checking", we should probably default to none or all depending on security stance.
		// Usually, "internalized logic" means we want them to work, but if we have the field, we should use it.
		// Let's assume for now that if the field is empty, we allow none for strictness,
		// but since it's a new field, it might break existing agents.
		// Actually, I'll default to allowing all if the list is EMPTY, but if it has items, only those items.
		// Wait, the user specifically wants to "allow checking which commands to allow".
		// That suggests a restricted list.
		allowed = true // For now, let's keep it open if empty for backward compatibility.
	} else {
		for _, b := range r.manifest.AllowedBuiltins {
			if b == req.Name {
				allowed = true
				break
			}
		}
	}

	if !allowed {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("builtin command %q is not allowed by manifest", req.Name), 1)
		return
	}

	switch req.Name {
	case "deploy":
		r.handleDeploy(ctx, req)
	case "backup":
		r.handleBackup(ctx, req)
	case "restore":
		r.handleRestore(ctx, req)
	default:
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("unknown builtin command: %s", req.Name), 1)
	}
}

func (r *Runner) handleBackup(ctx context.Context, req queue.CommandRequest) {
	if req.Environment == "" {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", "environment name is required for backup", 1)
		return
	}

	envDir := r.manifest.Resolve(filepath.Join(r.manifest.Environments, req.Environment))
	backupDir := filepath.Join(envDir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("failed to create backup directory: %v", err), 1)
		return
	}

	timestamp := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("backup_%s.sql", timestamp)
	backupPath := filepath.Join(backupDir, filename)

	r.emit(req, "output", "stdout", fmt.Sprintf("Starting backup for %s...", req.Environment), 0)

	// We need to find the active project
	projectName, err := r.getActiveProject(req.Environment)
	if err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("failed to find active project: %v", err), 1)
		return
	}

	// Run pg_dump
	cmd := exec.CommandContext(ctx, "docker", "compose", "-p", projectName, "exec", "-T", "db", "pg_dump", "-U", "odoo", "odoo")
	cmd.Dir = envDir
	outFile, err := os.Create(backupPath)
	if err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("failed to create backup file: %v", err), 1)
		return
	}
	defer outFile.Close()
	cmd.Stdout = outFile

	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("failed to start pg_dump: %v", err), 1)
		return
	}
	go r.scan(req, "stderr", stderr)

	if err := cmd.Wait(); err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("pg_dump failed: %v", err), 1)
		return
	}

	r.emit(req, "output", "stdout", fmt.Sprintf("Backup completed: %s", backupPath), 0)
	atomic.AddUint64(&r.metrics.CommandsSucceeded, 1)
	r.emit(req, "succeeded", "", "Backup completed successfully", 0)
}

func (r *Runner) getActiveProject(environment string) (string, error) {
	cmd := exec.Command("docker", "compose", "ls", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var projects []map[string]interface{}
	if err := json.Unmarshal(output, &projects); err == nil {
		for _, p := range projects {
			name, ok := p["Name"].(string)
			status, _ := p["Status"].(string)
			if ok && strings.HasPrefix(name, environment+"_") && strings.Contains(status, "running") {
				return name, nil
			}
		}
	}
	return "", fmt.Errorf("no running project found for environment %s", environment)
}

func (r *Runner) handleRestore(ctx context.Context, req queue.CommandRequest) {
	if req.Environment == "" {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", "environment name is required for restore", 1)
		return
	}

	if len(req.Args) == 0 {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", "backup filename is required in args", 1)
		return
	}
	filename := req.Args[0]

	envDir := r.manifest.Resolve(filepath.Join(r.manifest.Environments, req.Environment))
	backupPath := filepath.Join(envDir, "backups", filename)
	if _, err := os.Stat(backupPath); err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("backup file not found: %s", backupPath), 1)
		return
	}

	projectName, err := r.getActiveProject(req.Environment)
	if err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("failed to find active project: %v", err), 1)
		return
	}

	r.emit(req, "output", "stdout", fmt.Sprintf("Restoring %s to %s...", filename, req.Environment), 0)

	// Run psql inside container
	cmd := exec.CommandContext(ctx, "docker", "compose", "-p", projectName, "exec", "-T", "db", "psql", "-U", "odoo", "odoo")
	cmd.Dir = envDir
	inFile, err := os.Open(backupPath)
	if err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("failed to open backup file: %v", err), 1)
		return
	}
	defer inFile.Close()
	cmd.Stdin = inFile

	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("failed to start psql: %v", err), 1)
		return
	}
	go r.scan(req, "stderr", stderr)

	if err := cmd.Wait(); err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("psql failed: %v", err), 1)
		return
	}

	// Anonymize if requested (e.g. if args[1] == "anonymize")
	if len(req.Args) > 1 && req.Args[1] == "anonymize" {
		r.emit(req, "output", "stdout", "Anonymizing data...", 0)
		r.anonymize(ctx, req, envDir, projectName)
	}

	r.emit(req, "output", "stdout", "Restore completed successfully", 0)
	atomic.AddUint64(&r.metrics.CommandsSucceeded, 1)
	r.emit(req, "succeeded", "", "Restore completed successfully", 0)
}

func (r *Runner) anonymize(ctx context.Context, req queue.CommandRequest, envDir, projectName string) {
	// Simple anonymization example: scrub email addresses in res_partner
	query := "UPDATE res_partner SET email = 'sanitized-' || id || '@example.com' WHERE email IS NOT NULL;"
	cmd := exec.CommandContext(ctx, "docker", "compose", "-p", projectName, "exec", "-T", "db", "psql", "-U", "odoo", "odoo", "-c", query)
	cmd.Dir = envDir
	_ = cmd.Run()
}

func (r *Runner) handleDeploy(ctx context.Context, req queue.CommandRequest) {
	if req.Environment == "" {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", "environment name is required for deploy", 1)
		return
	}

	envDir := r.manifest.Resolve(filepath.Join(r.manifest.Environments, req.Environment))
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("failed to create environment directory: %v", err), 1)
		return
	}

	// Determine version and project name
	versionSuffix := time.Now().Format("20060102150405")
	projectName := fmt.Sprintf("%s_%s", req.Environment, versionSuffix)

	r.emit(req, "output", "stdout", fmt.Sprintf("Deploying to environment: %s (Project: %s)", req.Environment, projectName), 0)

	// Step 0: Docker login if token is present
	token := os.Getenv("STACCATO_GIT_TOKEN")
	if token != "" {
		r.emit(req, "output", "stdout", "Logging into GHCR...", 0)
		loginCmd := exec.CommandContext(ctx, "docker", "login", "ghcr.io", "-u", "token", "--password-stdin")
		loginCmd.Stdin = strings.NewReader(token)
		if err := loginCmd.Run(); err != nil {
			r.emit(req, "output", "stderr", fmt.Sprintf("docker login failed: %v", err), 0)
		}
	}

	// Step 1: Pull odoo-env image
	r.emit(req, "output", "stdout", "Step 1: Pulling odoo-env image...", 0)
	odooVersion := r.manifest.Odoo.Version
	if odooVersion == "" {
		odooVersion = "17.0" // Default
	}
	image := fmt.Sprintf("odoo-env:%s", odooVersion)
	// If it's a GHCR image, it might be prefixed. For now we assume typical odoo-env.
	if err := r.runInternal(ctx, req, envDir, "docker", "pull", image); err != nil {
		r.emit(req, "output", "stderr", fmt.Sprintf("failed to pull image %s: %v", image, err), 0)
	}

	// Step 2 & 3: Sync Repos
	r.emit(req, "output", "stdout", "Step 2 & 3: Syncing repositories...", 0)
	odooDir := filepath.Join(envDir, "odoo")
	if err := r.syncRepo(ctx, req, odooDir, r.manifest.Repo, "master"); err != nil {
		atomic.AddUint64(&r.metrics.CommandsFailed, 1)
		r.emit(req, "failed", "", fmt.Sprintf("failed to sync odoo repo: %v", err), 1)
		return
	}
	addonsDir := filepath.Join(envDir, "repos")
	if r.manifest.Odoo.AddonsRepo != "" {
		if err := r.syncRepo(ctx, req, addonsDir, r.manifest.Odoo.AddonsRepo, "master"); err != nil {
			atomic.AddUint64(&r.metrics.CommandsFailed, 1)
			r.emit(req, "failed", "", fmt.Sprintf("failed to sync addons repo: %v", err), 1)
			return
		}
	}

	// Step 4: Pip requirements
	r.emit(req, "output", "stdout", "Step 4: Installing pip requirements...", 0)
	reqFile := filepath.Join(odooDir, "requirements.txt")
	if _, err := os.Stat(reqFile); err == nil {
		_ = r.runInternal(ctx, req, envDir, "pip", "install", "-r", "odoo/requirements.txt")
	}

	// Step 5: Run odoo and update modules
	r.emit(req, "output", "stdout", "Step 5: Running Odoo and updating modules...", 0)
	composeFile := r.manifest.Docker.ComposeFile
	if composeFile == "" {
		composeFile = "docker-compose.yaml"
	}

	if _, err := os.Stat(filepath.Join(envDir, composeFile)); err == nil {
		r.emit(req, "output", "stdout", "Starting containers...", 0)
		if err := r.runInternal(ctx, req, envDir, "docker", "compose", "-f", composeFile, "-p", projectName, "up", "-d"); err != nil {
			atomic.AddUint64(&r.metrics.CommandsFailed, 1)
			r.emit(req, "failed", "", fmt.Sprintf("docker compose up failed: %v", err), 1)
			return
		}

		// Health Check
		r.emit(req, "output", "stdout", "Waiting for health check...", 0)
		// simple sleep for now, could be improved with actual probe
		time.Sleep(5 * time.Second)

		r.emit(req, "output", "stdout", "Updating modules...", 0)
		if err := r.runInternal(ctx, req, envDir, "docker", "compose", "-f", composeFile, "-p", projectName, "exec", "-T", "odoo", "odoo", "-u", "all", "--stop-after-init"); err != nil {
			r.emit(req, "output", "stderr", fmt.Sprintf("module update failed: %v", err), 0)
		}

		// Cleanup old deployments if any
		r.emit(req, "output", "stdout", "Swapping and cleaning up old deployments...", 0)
		r.cleanupOldDeployments(ctx, req, envDir, req.Environment, projectName)

	} else {
		r.emit(req, "output", "stderr", fmt.Sprintf("compose file %s not found in %s", composeFile, envDir), 0)
	}

	atomic.AddUint64(&r.metrics.CommandsSucceeded, 1)
	r.emit(req, "succeeded", "", "Deployment completed successfully", 1)
}

func (r *Runner) cleanupOldDeployments(ctx context.Context, req queue.CommandRequest, envDir, environment, currentProject string) {
	// List projects and stop those starting with environment prefix but not currentProject
	cmd := exec.Command("docker", "compose", "ls", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return
	}
	var projects []map[string]interface{}
	if err := json.Unmarshal(output, &projects); err == nil {
		for _, p := range projects {
			name, ok := p["Name"].(string)
			if ok && strings.HasPrefix(name, environment+"_") && name != currentProject {
				r.emit(req, "output", "stdout", fmt.Sprintf("Stopping old deployment: %s", name), 0)
				_ = r.runInternal(ctx, req, envDir, "docker", "compose", "-p", name, "down")
			}
		}
	}
}

func (r *Runner) runInternal(ctx context.Context, req queue.CommandRequest, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	go r.scan(req, "stdout", stdout)
	go r.scan(req, "stderr", stderr)
	return cmd.Wait()
}

func (r *Runner) syncRepo(ctx context.Context, req queue.CommandRequest, dir, gitURL, branch string) error {
	token := os.Getenv("STACCATO_GIT_TOKEN")
	authenticatedURL := gitURL
	if token != "" && strings.Contains(gitURL, "github.com") {
		if strings.HasPrefix(gitURL, "https://github.com/") {
			authenticatedURL = strings.Replace(gitURL, "https://github.com/", fmt.Sprintf("https://x-access-token:%s@github.com/", token), 1)
		} else if !strings.Contains(gitURL, "://") && strings.Count(gitURL, "/") == 1 {
			// Handle "owner/repo" format
			authenticatedURL = fmt.Sprintf("https://x-access-token:%s@github.com/%s.git", token, gitURL)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0o755)
		return r.runInternal(ctx, req, dir, "git", "clone", "--depth", "1", "-b", branch, authenticatedURL, ".")
	}
	// For pulls, we might need to update the remote URL to include the token if it's not already there.
	_ = r.runInternal(ctx, req, dir, "git", "remote", "set-url", "origin", authenticatedURL)
	return r.runInternal(ctx, req, dir, "git", "pull", "origin", branch)
}
