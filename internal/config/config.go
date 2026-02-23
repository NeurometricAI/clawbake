package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port               string
	DatabaseURL        string
	OIDCIssuer         string
	OIDCClientID       string
	OIDCClientSecret   string
	OIDCRedirectURL    string
	SessionSecret      string
	SlackBotToken      string
	SlackSigningSecret string
	KubeNamespace      string
	IngressDomain      string
	IngressScheme      string
	IngressPort        string

	InstanceDefaultImage         string
	InstanceDefaultCPURequest    string
	InstanceDefaultMemoryRequest string
	InstanceDefaultCPULimit      string
	InstanceDefaultMemoryLimit   string
	InstanceDefaultStorageSize   string
}

func Load() *Config {
	return &Config{
		Port:               envOrDefault("PORT", "8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		OIDCIssuer:         os.Getenv("OIDC_ISSUER"),
		OIDCClientID:       os.Getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret:   os.Getenv("OIDC_CLIENT_SECRET"),
		OIDCRedirectURL:    os.Getenv("OIDC_REDIRECT_URL"),
		SessionSecret:      os.Getenv("SESSION_SECRET"),
		SlackBotToken:      os.Getenv("SLACK_BOT_TOKEN"),
		SlackSigningSecret: os.Getenv("SLACK_SIGNING_SECRET"),
		KubeNamespace:      envOrDefault("KUBE_NAMESPACE", "clawbake"),
		IngressDomain:      envOrDefault("INGRESS_DOMAIN", "claw.example.com"),
		IngressScheme:      envOrDefault("INGRESS_SCHEME", "https"),
		IngressPort:        os.Getenv("INGRESS_PORT"),

		InstanceDefaultImage:         os.Getenv("INSTANCE_DEFAULT_IMAGE"),
		InstanceDefaultCPURequest:    os.Getenv("INSTANCE_DEFAULT_CPU_REQUEST"),
		InstanceDefaultMemoryRequest: os.Getenv("INSTANCE_DEFAULT_MEMORY_REQUEST"),
		InstanceDefaultCPULimit:      os.Getenv("INSTANCE_DEFAULT_CPU_LIMIT"),
		InstanceDefaultMemoryLimit:   os.Getenv("INSTANCE_DEFAULT_MEMORY_LIMIT"),
		InstanceDefaultStorageSize:   os.Getenv("INSTANCE_DEFAULT_STORAGE_SIZE"),
	}
}

// BuildIngressURL constructs a full URL from scheme, host, and optional port.
func BuildIngressURL(scheme, host, port string) string {
	if port != "" {
		return fmt.Sprintf("%s://%s:%s", scheme, host, port)
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
