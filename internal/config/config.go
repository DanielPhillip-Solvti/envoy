package config

import "os"

type Platform struct {
	HTTPAddr  string
	NATSURL   string
	NATSNKey  string
	ObjectDir string
}

type Agent struct {
	ManifestPath string
	NATSURL      string
	NATSNKey     string
	ObjectDir    string
}

func PlatformFromEnv() Platform {
	return Platform{
		HTTPAddr:  env("ENVOY_HTTP_ADDR", ":8080"),
		NATSURL:   env("ENVOY_NATS_URL", "nats://127.0.0.1:4222"),
		NATSNKey:  os.Getenv("ENVOY_NATS_NKEY"),
		ObjectDir: env("ENVOY_OBJECT_DIR", "var/objects"),
	}
}

func AgentFromEnv() Agent {
	return Agent{
		ManifestPath: env("ENVOY_MANIFEST", "examples/agent.manifest.yaml"),
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
