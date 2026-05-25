package config

import "os"

type Platform struct {
	HTTPAddr           string
	NATSURL            string
	NATSNKey           string
	ObjectDir          string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubCallbackURL  string
	SessionSecure      bool
}

type Agent struct {
	ManifestPath string
	NATSURL      string
	NATSNKey     string
	ObjectDir    string
}

func PlatformFromEnv() Platform {
	return Platform{
		HTTPAddr:           env("ENVOY_HTTP_ADDR", ":8080"),
		NATSURL:            env("ENVOY_NATS_URL", "nats://127.0.0.1:4222"),
		NATSNKey:           os.Getenv("ENVOY_NATS_NKEY"),
		ObjectDir:          env("ENVOY_OBJECT_DIR", "var/objects"),
		GitHubClientID:     os.Getenv("ENVOY_GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("ENVOY_GITHUB_CLIENT_SECRET"),
		GitHubCallbackURL:  env("ENVOY_GITHUB_CALLBACK_URL", "http://localhost:8080/auth/github/callback"),
		SessionSecure:      env("ENVOY_SESSION_SECURE", "false") == "true",
	}
}

func AgentFromEnv() Agent {
	return Agent{
		ManifestPath: env("ENVOY_MANIFEST", "test/vm/agent.manifest.yaml"),
		NATSURL:      env("ENVOY_NATS_URL", "nats://127.0.0.1:4222"),
		NATSNKey:     os.Getenv("ENVOY_NATS_NKEY"),
		ObjectDir:    env("ENVOY_OBJECT_DIR", "var/objects"),
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
