package queue

import "testing"

func TestSubjects(t *testing.T) {
	cases := map[string]string{
		SubjectCommand("agent-1"):           "envoy.command.agent-1",
		SubjectCommandEvent("cmd-1"):        "envoy.command.event.cmd-1",
		SubjectLogs("agent-1", "feature-x"): "envoy.logs.agent-1.feature-x",
		SubjectLogRequest("agent-1"):        "envoy.log.request.agent-1",
		SubjectFileRequest("agent-1"):       "envoy.file.request.agent-1",
		SubjectFileResponse("file-1"):       "envoy.file.response.file-1",
		SubjectAgentRegister:                "envoy.agent.register",
		SubjectAgentHeartbeat:               "envoy.agent.heartbeat",
	}

	for got, want := range cases {
		if got != want {
			t.Fatalf("subject = %q, want %q", got, want)
		}
	}
}
