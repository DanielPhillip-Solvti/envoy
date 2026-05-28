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
	GitHubAppID        string
	GitHubPrivateKey   string
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
		HTTPAddr:           env("STACCATO_HTTP_ADDR", ":8080"),
		NATSURL:            env("STACCATO_NATS_URL", "nats://127.0.0.1:4222"),
		NATSNKey:           os.Getenv("STACCATO_NATS_NKEY"),
		ObjectDir:          env("STACCATO_OBJECT_DIR", "var/objects"),
		GitHubClientID:     os.Getenv("STACCATO_GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("STACCATO_GITHUB_CLIENT_SECRET"),
		GitHubCallbackURL:  env("STACCATO_GITHUB_CALLBACK_URL", "http://localhost:8080/auth/github/callback"),
		GitHubAppID:        os.Getenv("STACCATO_GITHUB_APP_ID"),
		GitHubPrivateKey:   os.Getenv("STACCATO_GITHUB_PRIVATE_KEY"),
		SessionSecure:      env("STACCATO_SESSION_SECURE", "false") == "true",
	}
}

func AgentFromEnv() Agent {
	return Agent{
		ManifestPath: env("STACCATO_MANIFEST", "test/vm/agent.manifest.yaml"),
		NATSURL:      env("STACCATO_NATS_URL", "nats://127.0.0.1:4222"),
		NATSNKey:     os.Getenv("STACCATO_NATS_NKEY"),
		ObjectDir:    env("STACCATO_OBJECT_DIR", "var/objects"),
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
