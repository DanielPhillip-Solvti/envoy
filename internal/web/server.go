package web

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/example/envoy/internal/platform"
	"github.com/example/envoy/internal/queue"
)

type Server struct {
	bus   *queue.Bus
	state *platform.State
}

func NewServer(bus *queue.Bus, state *platform.State) http.Handler {
	s := &Server{bus: bus, state: state}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.agents)
	mux.HandleFunc("GET /agents", s.agents)
	mux.HandleFunc("GET /agents/{agentID}", s.agent)
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
	_ = agentsTemplate.Execute(w, s.state.Agents())
}

func (s *Server) agent(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.state.Agent(r.PathValue("agentID"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	data := struct {
		Agent platform.AgentView
		Files []queue.FileResponse
	}{Agent: agent, Files: s.state.FileResponses()}
	_ = agentTemplate.Execute(w, data)
}

func (s *Server) environment(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.state.Agent(r.PathValue("agentID"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	data := struct {
		Agent       platform.AgentView
		Environment string
		Logs        []queue.LogEvent
	}{
		Agent:       agent,
		Environment: r.PathValue("environment"),
		Logs:        s.state.Logs(r.PathValue("agentID"), r.PathValue("environment")),
	}
	_ = environmentTemplate.Execute(w, data)
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
	http.Redirect(w, r, "/agents/"+agentID, http.StatusSeeOther)
}

func (s *Server) commandEvents(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("commandID")
	_ = commandEventsTemplate.Execute(w, s.state.CommandEvents(commandID))
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

var agentsTemplate = template.Must(template.New("agents").Parse(`<!doctype html>
<html>
<head>
  <title>Envoy</title>
  <script src="https://unpkg.com/htmx.org@1.9.12"></script>
  <style>
    body { font-family: system-ui, sans-serif; margin: 32px; background: #f7f7f4; color: #1f2428; }
    .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 16px; }
    .card { background: white; border: 1px solid #ddd; border-radius: 8px; padding: 16px; }
    .muted { color: #667; }
  </style>
</head>
<body>
  <h1>Envoy Agents</h1>
  <div class="grid">
    {{ range . }}
      <a class="card" href="/agents/{{ .Registration.AgentID }}">
        <h2>{{ .Registration.Name }}</h2>
        <p class="muted">{{ .Registration.Repo }}</p>
        <p>{{ len .Heartbeat.Environments }} environments</p>
      </a>
    {{ else }}
      <p>No agents have registered yet.</p>
    {{ end }}
  </div>
</body>
</html>`))

var agentTemplate = template.Must(template.New("agent").Parse(`<!doctype html>
<html><body>
<h1>{{ .Agent.Registration.Name }}</h1>
  <section>
    <h3>Commands</h3>
    <ul>{{ range .Agent.Registration.Scripts }}
      <li>
        <form method="post" action="/agents/{{ $.Agent.Registration.AgentID }}/commands">
          <input type="hidden" name="scope" value="{{ .Scope }}">
          <input type="hidden" name="name" value="{{ .Name }}">
          {{ if eq .Scope "env" }}<input name="environment" placeholder="environment">{{ end }}
          {{ range .Args }}<input name="args" placeholder="{{ . }}">{{ end }}
          <button type="submit">{{ .Scope }}: {{ .Name }}</button>
        </form>
      </li>
    {{ end }}</ul>
    <h3>Files</h3>
    <ul>{{ range .Agent.Registration.Files }}
      <li>
        <form method="post" action="/agents/{{ $.Agent.Registration.AgentID }}/files/{{ .Key }}">
          <button type="submit">{{ .Key }}</button>
        </form>
      </li>
    {{ end }}</ul>
    <h3>File Responses</h3>
    <ul>{{ range .Files }}<li>{{ .FileKey }}: {{ if .Error }}{{ .Error }}{{ else }}{{ .ObjectURL }} ({{ .Size }} bytes){{ end }}</li>{{ end }}</ul>
    <h3>Environments</h3>
    <ul>{{ range .Agent.Heartbeat.Environments }}<li><a href="/agents/{{ $.Agent.Registration.AgentID }}/env/{{ .Name }}">{{ .Name }}</a></li>{{ end }}</ul>
    <h3>Consumption</h3>
    <pre>{{ printf "%+v" .Agent.Heartbeat.Consumption }}</pre>
  </section>
</body></html>`))

var environmentTemplate = template.Must(template.New("environment").Parse(`<!doctype html>
<html><body>
<h1>{{ .Agent.Registration.Name }} / {{ .Environment }}</h1>
<nav>
  <a href="/agents/{{ .Agent.Registration.AgentID }}">Commands</a>
  <a href="/commands/example/events">Events</a>
</nav>
<form method="post" action="/agents/{{ .Agent.Registration.AgentID }}/env/{{ .Environment }}/logs">
  <button type="submit">Refresh logs</button>
</form>
<h2>Services</h2>
{{ range .Agent.Heartbeat.Environments }}
  {{ if eq .Name $.Environment }}
    <ul>{{ range $name, $status := .Services }}<li>{{ $name }}: {{ $status }}</li>{{ end }}</ul>
  {{ end }}
{{ end }}
<h2>Logs</h2>
<pre>{{ range .Logs }}{{ .Line }}
{{ end }}</pre>
</body></html>`))

var commandEventsTemplate = template.Must(template.New("events").Parse(`{{ range . }}
<div><strong>{{ .Status }}</strong> {{ .Stream }} {{ .Message }}</div>
{{ end }}`))
