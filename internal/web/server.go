package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"sort"
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
	Agent               platform.AgentView
	SelectedEnvironment string
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
	s := &Server{bus: bus, state: state, tmpl: tmpl}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
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
	mux.HandleFunc("GET /commands/{commandID}/events", s.commandEvents)
	mux.HandleFunc("GET /auth/github/login", s.githubLogin)
	mux.HandleFunc("GET /auth/github/callback", s.githubCallback)
	return mux
}

func (s *Server) agents(w http.ResponseWriter, r *http.Request) {
	agents := s.state.Agents()
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Registration.Name < agents[j].Registration.Name
	})
	s.renderLayout(w, http.StatusOK, "Envoy | Home", "home_page", homePageData{Agents: agents})
}

func (s *Server) agent(w http.ResponseWriter, r *http.Request) {
	data, ok := s.agentData(r.PathValue("agentID"), r.URL.Query().Get("environment"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderLayout(w, http.StatusOK, "Envoy | "+data.Agent.Registration.Name, "agent_page", data)
}

func (s *Server) environment(w http.ResponseWriter, r *http.Request) {
	data, ok := s.agentData(r.PathValue("agentID"), r.PathValue("environment"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderLayout(w, http.StatusOK, "Envoy | "+data.Agent.Registration.Name, "agent_page", data)
}

func (s *Server) agentTabEvents(w http.ResponseWriter, r *http.Request) {
	data, ok := s.agentData(r.PathValue("agentID"), r.URL.Query().Get("environment"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderTemplate(w, http.StatusOK, "tab_events", data)
}

func (s *Server) agentTabLogs(w http.ResponseWriter, r *http.Request) {
	data, ok := s.agentData(r.PathValue("agentID"), r.URL.Query().Get("environment"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderTemplate(w, http.StatusOK, "tab_logs", data)
}

func (s *Server) agentTabCommands(w http.ResponseWriter, r *http.Request) {
	data, ok := s.agentData(r.PathValue("agentID"), r.URL.Query().Get("environment"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderTemplate(w, http.StatusOK, "tab_commands", data)
}

func (s *Server) agentTabFiles(w http.ResponseWriter, r *http.Request) {
	data, ok := s.agentData(r.PathValue("agentID"), r.URL.Query().Get("environment"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderTemplate(w, http.StatusOK, "tab_files", data)
}

func (s *Server) requestLogs(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	environment := r.PathValue("environment")
	if _, ok := s.state.Agent(agentID); !ok {
		http.NotFound(w, r)
		return
	}
	req := queue.LogRequest{
		RequestID:   requestID(),
		AgentID:     agentID,
		Environment: environment,
		Tail:        100,
		AskedAt:     time.Now().UTC(),
	}
	if err := s.bus.PublishJSON(queue.SubjectLogRequest(agentID), req); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if isHX(r) {
		data, ok := s.agentData(agentID, environment)
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.renderTemplate(w, http.StatusOK, "tab_logs", data)
		return
	}
	http.Redirect(w, r, "/agents/"+agentID+"/env/"+environment, http.StatusSeeOther)
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
		data, ok := s.agentData(agentID, r.URL.Query().Get("environment"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.renderTemplate(w, http.StatusOK, "tab_files", data)
		return
	}
	http.Redirect(w, r, "/agents/"+agentID, http.StatusSeeOther)
}

func (s *Server) commandEvents(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("commandID")
	s.renderTemplate(w, http.StatusOK, "command_events", s.state.CommandEvents(commandID))
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

func (s *Server) agentData(agentID, selectedEnvironment string) (agentPageData, bool) {
	agent, ok := s.state.Agent(agentID)
	if !ok {
		return agentPageData{}, false
	}

	if selectedEnvironment == "" && len(agent.Heartbeat.Environments) > 0 {
		selectedEnvironment = agent.Heartbeat.Environments[0].Name
	}

	return agentPageData{
		Agent:               agent,
		SelectedEnvironment: selectedEnvironment,
		Events:              s.state.AgentEvents(agentID),
		Files:               filterFileResponsesByAgent(s.state.FileResponses(), agentID),
		Logs:                s.state.Logs(agentID, selectedEnvironment),
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
