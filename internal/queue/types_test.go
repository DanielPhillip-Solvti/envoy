package queue

import "testing"

func TestSubjects(t *testing.T) {
	cases := map[string]string{
		SubjectCommand("agent-1"):           "staccato.command.agent-1",
		SubjectCommandEvent("cmd-1"):        "staccato.command.event.cmd-1",
		SubjectLogs("agent-1", "feature-x"): "staccato.logs.agent-1.feature-x",
		SubjectLogRequest("agent-1"):        "staccato.log.request.agent-1",
		SubjectFileRequest("agent-1"):       "staccato.file.request.agent-1",
		SubjectFileResponse("file-1"):       "staccato.file.response.file-1",
		SubjectAgentRegister:                "staccato.agent.register",
		SubjectAgentHeartbeat:               "staccato.agent.heartbeat",
	}

	for got, want := range cases {
		if got != want {
			t.Fatalf("subject = %q, want %q", got, want)
		}
	}
}
