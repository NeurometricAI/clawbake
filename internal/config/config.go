package config

import "os"

type Config struct {
	Port               string
	DatabaseURL        string
	BaseURL            string
	OIDCIssuer         string
	OIDCClientID       string
	OIDCClientSecret   string
	OIDCRedirectURL    string
	SessionSecret      string
	SlackBotToken      string
	SlackSigningSecret string
	KubeNamespace      string

	InstanceDefaultImage         string
	InstanceDefaultCPURequest    string
	InstanceDefaultMemoryRequest string
	InstanceDefaultCPULimit      string
	InstanceDefaultMemoryLimit   string
	InstanceDefaultStorageSize   string
	InstanceDefaultGatewayConfig string

	TUIEnabled   bool
	ShellEnabled bool
}

func Load() *Config {
	return &Config{
		Port:               envOrDefault("PORT", "8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		BaseURL:            os.Getenv("BASE_URL"),
		OIDCIssuer:         os.Getenv("OIDC_ISSUER"),
		OIDCClientID:       os.Getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret:   os.Getenv("OIDC_CLIENT_SECRET"),
		OIDCRedirectURL:    os.Getenv("OIDC_REDIRECT_URL"),
		SessionSecret:      os.Getenv("SESSION_SECRET"),
		SlackBotToken:      os.Getenv("SLACK_BOT_TOKEN"),
		SlackSigningSecret: os.Getenv("SLACK_SIGNING_SECRET"),
		KubeNamespace:      envOrDefault("KUBE_NAMESPACE", "clawbake"),

		InstanceDefaultImage:         os.Getenv("INSTANCE_DEFAULT_IMAGE"),
		InstanceDefaultCPURequest:    os.Getenv("INSTANCE_DEFAULT_CPU_REQUEST"),
		InstanceDefaultMemoryRequest: os.Getenv("INSTANCE_DEFAULT_MEMORY_REQUEST"),
		InstanceDefaultCPULimit:      os.Getenv("INSTANCE_DEFAULT_CPU_LIMIT"),
		InstanceDefaultMemoryLimit:   os.Getenv("INSTANCE_DEFAULT_MEMORY_LIMIT"),
		InstanceDefaultStorageSize:   os.Getenv("INSTANCE_DEFAULT_STORAGE_SIZE"),
		InstanceDefaultGatewayConfig: os.Getenv("INSTANCE_DEFAULT_GATEWAY_CONFIG"),

		TUIEnabled:   os.Getenv("TUI_ENABLED") != "false",
		ShellEnabled: os.Getenv("SHELL_ENABLED") != "false",
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
