package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/example/envoy/internal/platform"
	"github.com/example/envoy/internal/queue"
)

type Server struct {
	bus   *queue.Bus
	state *platform.State
	tmpl  *template.Template
}

type layoutView struct {
	Title string
	Body  string
	Data  any
}

type homePageData struct {
	Agents []platform.AgentView
}

type agentPageData struct {
	AgentID             string
	AgentName           string
	Agent               platform.AgentView
	SelectedEnvironment string
	SelectedService     string
	Services            []string
	Events              []queue.CommandEvent
	Files               []queue.FileResponse
	Logs                []queue.LogEvent
}

//go:embed templates/*.html templates/partials/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

func NewServer(bus *queue.Bus, state *platform.State) http.Handler {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html", "templates/partials/*.html"))
	staticSubFS := mustSubFS(staticFS, "static")
	s := &Server{bus: bus, state: state, tmpl: tmpl}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS))))
	mux.HandleFunc("GET /", s.agents)
	mux.HandleFunc("GET /agents", s.agents)
	mux.HandleFunc("GET /agents/{agentID}", s.agent)
	mux.HandleFunc("GET /agents/{agentID}/tabs/events", s.agentTabEvents)
	mux.HandleFunc("GET /agents/{agentID}/tabs/logs", s.agentTabLogs)
	mux.HandleFunc("GET /agents/{agentID}/tabs/commands", s.agentTabCommands)
	mux.HandleFunc("GET /agents/{agentID}/tabs/files", s.agentTabFiles)
	mux.HandleFunc("GET /agents/{agentID}/env/{environment}", s.environment)
	mux.HandleFunc("POST /agents/{agentID}/env/{environment}/logs", s.requestLogs)
	mux.HandleFunc("POST /agents/{agentID}/commands", s.createCommand)
	mux.HandleFunc("POST /agents/{agentID}/files/{fileKey}", s.requestFile)
	mux.HandleFunc("GET /file-requests/{requestID}", s.fileRequestStatus)
	mux.HandleFunc("GET /downloads/{requestID}", s.downloadFile)
	mux.HandleFunc("GET /commands/{commandID}/events", s.commandEvents)
	mux.HandleFunc("GET /auth/github/login", s.githubLogin)
	mux.HandleFunc("GET /auth/github/callback", s.githubCallback)
	return mux
}

func (s *Server) agents(w http.ResponseWriter, r *http.Request) {
	agents := s.state.Agents()
	for _, agent := range agents {
		s.requestCapabilitiesIfMissing(agent)
	}
	sort.Slice(agents, func(i, j int) bool {
		return agentDisplayName(agents[i]) < agentDisplayName(agents[j])
	})
	s.renderLayout(w, http.StatusOK, "Envoy | Home", "home_page", homePageData{Agents: agents})
}

func (s *Server) agent(w http.ResponseWriter, r *http.Request) {
	data, ok := s.agentData(r.PathValue("agentID"), r.URL.Query().Get("environment"), r.URL.Query().Get("service"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderLayout(w, http.StatusOK, "Envoy | "+data.AgentName, "agent_page", data)
}

func (s *Server) environment(w http.ResponseWriter, r *http.Request) {
	data, ok := s.agentData(r.PathValue("agentID"), r.PathValue("environment"), r.URL.Query().Get("service"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderLayout(w, http.StatusOK, "Envoy | "+data.AgentName, "agent_page", data)
}

func (s *Server) agentTabEvents(w http.ResponseWriter, r *http.Request) {
	data, ok := s.agentData(r.PathValue("agentID"), r.URL.Query().Get("environment"), r.URL.Query().Get("service"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderTemplate(w, http.StatusOK, "tab_events", data)
}

func (s *Server) agentTabLogs(w http.ResponseWriter, r *http.Request) {
	data, ok := s.agentData(r.PathValue("agentID"), r.URL.Query().Get("environment"), r.URL.Query().Get("service"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderTemplate(w, http.StatusOK, "tab_logs", data)
}

func (s *Server) agentTabCommands(w http.ResponseWriter, r *http.Request) {
	data, ok := s.agentData(r.PathValue("agentID"), r.URL.Query().Get("environment"), r.URL.Query().Get("service"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderTemplate(w, http.StatusOK, "tab_commands", data)
}

func (s *Server) agentTabFiles(w http.ResponseWriter, r *http.Request) {
	data, ok := s.agentData(r.PathValue("agentID"), r.URL.Query().Get("environment"), r.URL.Query().Get("service"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderTemplate(w, http.StatusOK, "tab_files", data)
}

func (s *Server) requestLogs(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	environment := r.PathValue("environment")
	selectedService := r.FormValue("service")
	if _, ok := s.state.Agent(agentID); !ok {
		http.NotFound(w, r)
		return
	}
	req := queue.LogRequest{
		RequestID:   requestID(),
		AgentID:     agentID,
		Environment: environment,
		Service:     selectedService,
		Tail:        100,
		AskedAt:     time.Now().UTC(),
	}
	if err := s.bus.PublishJSON(queue.SubjectLogRequest(agentID), req); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if isHX(r) {
		data, ok := s.agentData(agentID, environment, selectedService)
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.renderTemplate(w, http.StatusOK, "tab_logs", data)
		return
	}
	redirectPath := "/agents/" + agentID + "/env/" + environment
	if strings.TrimSpace(selectedService) != "" {
		redirectPath += "?service=" + url.QueryEscape(selectedService)
	}
	http.Redirect(w, r, redirectPath, http.StatusSeeOther)
}

func (s *Server) createCommand(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	if _, ok := s.state.Agent(agentID); !ok {
		http.NotFound(w, r)
		return
	}
	req := queue.CommandRequest{
		CommandID:   commandID(),
		AgentID:     agentID,
		Scope:       r.FormValue("scope"),
		Name:        r.FormValue("name"),
		Environment: r.FormValue("environment"),
		Args:        r.Form["args"],
		RequestedBy: "local-dev",
		RequestedAt: time.Now().UTC(),
	}
	if req.Scope == "" || req.Name == "" {
		http.Error(w, "scope and name are required", http.StatusBadRequest)
		return
	}
	s.state.ApplyCommandRequest(req)
	if err := s.bus.PublishJSON(queue.SubjectCommand(agentID), req); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if isHX(r) {
		s.renderTemplate(w, http.StatusOK, "command_status", struct {
			CommandID string
		}{CommandID: req.CommandID})
		return
	}
	http.Redirect(w, r, "/commands/"+req.CommandID+"/events", http.StatusSeeOther)
}

func (s *Server) requestFile(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	if _, ok := s.state.Agent(agentID); !ok {
		http.NotFound(w, r)
		return
	}
	req := queue.FileRequest{
		RequestID: requestID(),
		AgentID:   agentID,
		FileKey:   r.PathValue("fileKey"),
		AskedAt:   time.Now().UTC(),
	}
	if err := s.bus.PublishJSON(queue.SubjectFileRequest(agentID), req); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if isHX(r) {
		s.renderTemplate(w, http.StatusOK, "file_request_status", fileRequestStatusData{
			RequestID: req.RequestID,
			FileKey:   req.FileKey,
			Pending:   true,
		})
		return
	}
	http.Redirect(w, r, "/agents/"+agentID, http.StatusSeeOther)
}

func (s *Server) fileRequestStatus(w http.ResponseWriter, r *http.Request) {
	requestID := r.PathValue("requestID")
	response, ok := s.state.FileResponse(requestID)
	if !ok {
		s.renderTemplate(w, http.StatusOK, "file_request_status", fileRequestStatusData{
			RequestID: requestID,
			Pending:   true,
		})
		return
	}

	s.renderTemplate(w, http.StatusOK, "file_request_status", fileRequestStatusData{
		RequestID: requestID,
		FileKey:   response.FileKey,
		FileName:  response.FileName,
		Error:     response.Error,
		Pending:   false,
	})
}

func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	requestID := r.PathValue("requestID")
	response, ok := s.state.FileResponse(requestID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if response.Error != "" {
		http.Error(w, response.Error, http.StatusBadGateway)
		return
	}
	if !strings.HasPrefix(response.ObjectURL, "file://") {
		http.Error(w, "download is not available for this object URL", http.StatusNotImplemented)
		return
	}

	pathValue := strings.TrimPrefix(response.ObjectURL, "file://")
	decodedPath, err := url.PathUnescape(pathValue)
	if err != nil {
		http.Error(w, "invalid file path", http.StatusBadRequest)
		return
	}

	file, err := os.Open(decodedPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer file.Close()

	name := r.URL.Query().Get("name")
	if strings.TrimSpace(name) == "" {
		name = response.FileName
	}
	if strings.TrimSpace(name) == "" {
		name = response.FileKey
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(decodedPath)
	}

	name = withExtensionFallback(name, response.FileName, filepath.Base(decodedPath))

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	http.ServeContent(w, r, name, stat.ModTime(), file)
}

func (s *Server) commandEvents(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("commandID")
	s.renderTemplate(w, http.StatusOK, "command_events", s.state.CommandEvents(commandID))
}

type fileRequestStatusData struct {
	RequestID string
	FileKey   string
	FileName  string
	Error     string
	Pending   bool
}

func (s *Server) githubLogin(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "GitHub OAuth is not configured in this scaffold", http.StatusNotImplemented)
}

func (s *Server) githubCallback(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "GitHub OAuth is not configured in this scaffold", http.StatusNotImplemented)
}

func commandID() string {
	return fmt.Sprintf("cmd-%d", time.Now().UnixNano())
}

func requestID() string {
	return fmt.Sprintf("file-%d", time.Now().UnixNano())
}

func (s *Server) agentData(agentID, selectedEnvironment, selectedService string) (agentPageData, bool) {
	agent, ok := s.state.Agent(agentID)
	if !ok {
		return agentPageData{}, false
	}
	s.requestCapabilitiesIfMissing(agent)

	if agentID == "" {
		agentID = agent.Heartbeat.AgentID
	}
	if agentID == "" {
		agentID = agent.Registration.AgentID
	}
	agentName := agentDisplayName(agent)

	if selectedEnvironment == "" && len(agent.Heartbeat.Environments) > 0 {
		selectedEnvironment = agent.Heartbeat.Environments[0].Name
	}

	services := servicesForEnvironment(agent.Heartbeat.Environments, selectedEnvironment)
	if selectedService != "" && !containsService(services, selectedService) {
		selectedService = ""
	}

	return agentPageData{
		AgentID:             agentID,
		AgentName:           agentName,
		Agent:               agent,
		SelectedEnvironment: selectedEnvironment,
		SelectedService:     selectedService,
		Services:            services,
		Events:              s.state.AgentEvents(agentID),
		Files:               filterFileResponsesByAgent(s.state.FileResponses(), agentID),
		Logs:                filterLogsByService(s.state.Logs(agentID, selectedEnvironment), selectedService),
	}, true
}

func (s *Server) renderLayout(w http.ResponseWriter, status int, title, body string, data any) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, "layout", layoutView{Title: title, Body: body, Data: data}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) renderTemplate(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func filterFileResponsesByAgent(responses []queue.FileResponse, agentID string) []queue.FileResponse {
	result := make([]queue.FileResponse, 0)
	for _, response := range responses {
		if response.AgentID == agentID {
			result = append(result, response)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SentAt.After(result[j].SentAt)
	})
	return result
}

func isHX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func servicesForEnvironment(envs []queue.EnvironmentStatus, environment string) []string {
	for _, env := range envs {
		if env.Name != environment {
			continue
		}
		services := make([]string, 0, len(env.Services))
		for name := range env.Services {
			services = append(services, name)
		}
		sort.Strings(services)
		return services
	}
	return nil
}

func containsService(services []string, selected string) bool {
	for _, service := range services {
		if service == selected {
			return true
		}
	}
	return false
}

func filterLogsByService(logs []queue.LogEvent, service string) []queue.LogEvent {
	if strings.TrimSpace(service) == "" {
		return logs
	}
	filtered := make([]queue.LogEvent, 0, len(logs))
	candidates := serviceMatchCandidates(service)
	for _, entry := range logs {
		line := strings.ToLower(entry.Line)
		for _, candidate := range candidates {
			if strings.Contains(line, candidate) {
				filtered = append(filtered, entry)
				break
			}
		}
	}
	return filtered
}

func serviceMatchCandidates(service string) []string {
	value := strings.ToLower(strings.TrimSpace(service))
	if value == "" {
		return nil
	}
	candidates := []string{value}
	parts := strings.Split(value, "-")
	if len(parts) > 2 {
		candidates = append(candidates, strings.Join(parts[1:], "-"))
	}
	if len(parts) > 1 {
		candidates = append(candidates, parts[1])
	}
	unique := make(map[string]struct{})
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, exists := unique[candidate]; exists {
			continue
		}
		unique[candidate] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

func agentDisplayName(agent platform.AgentView) string {
	if agent.Registration.Name != "" {
		return agent.Registration.Name
	}
	if agent.Heartbeat.AgentID != "" {
		return agent.Heartbeat.AgentID
	}
	if agent.Registration.AgentID != "" {
		return agent.Registration.AgentID
	}
	return "unregistered-agent"
}

func mustSubFS(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

func (s *Server) requestCapabilitiesIfMissing(agent platform.AgentView) {
	if len(agent.Registration.Scripts) > 0 || len(agent.Registration.Files) > 0 {
		return
	}
	agentID := agent.Heartbeat.AgentID
	if agentID == "" {
		agentID = agent.Registration.AgentID
	}
	if agentID == "" {
		return
	}
	_ = s.bus.PublishJSON(queue.SubjectCapabilityRequest(agentID), queue.CapabilityRequest{
		AgentID:     agentID,
		RequestedAt: time.Now().UTC(),
	})
}

func withExtensionFallback(name, preferredName, sourceBase string) string {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		cleanName = strings.TrimSpace(preferredName)
	}
	if cleanName == "" {
		cleanName = strings.TrimSpace(sourceBase)
	}
	if cleanName == "" {
		return "download"
	}

	if filepath.Ext(cleanName) != "" {
		return cleanName
	}

	ext := filepath.Ext(strings.TrimSpace(preferredName))
	if ext == "" {
		ext = filepath.Ext(strings.TrimSpace(sourceBase))
	}
	if ext != "" {
		return cleanName + ext
	}

	return cleanName
}
