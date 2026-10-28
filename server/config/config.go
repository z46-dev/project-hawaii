package config

import (
	"fmt"

	"github.com/z46-dev/goconf"
)

type Config struct {
	WebServer struct {
		BindAddress     string   `toml:"bind_address" default:"[::]:8000" validate:"required"`
		AcceptedOrigins []string `toml:"accepted_origins" default:"[\"http://localhost:3000\"]" validate:"required"`
		TrustedProxies  []string `toml:"trusted_proxies"`
	} `toml:"web_server"`
	Database struct {
		FilePath string `toml:"file_path" default:"./hawaii-sqlite.db" validate:"required"`
	} `toml:"database"`
	LLMServer struct {
		OpenAI_Like_API_URL string `toml:"openai_like_api_url" default:"http://localhost:1234/v1" validate:"required"`
		OpenAI_Like_API_Key string `toml:"openai_like_api_key" default:"your_key" validate:"required"`
	} `toml:"llm_server"`
	Authentication struct {
		FrontendURL   string  `toml:"frontend_url" default:"http://localhost:3000"`
		SessionSecret string  `toml:"session_secret" default:""`
		Realms        []Realm `toml:"realm"`
	} `toml:"authentication"`
}

type Realm struct {
	ID       string            `toml:"id"`
	Name     string            `toml:"name"`
	Type     string            `toml:"type"`
	Settings map[string]string `toml:"settings"`
}

var Conf Config

// Init loads configuration from config.toml.
func Init() (err error) {
	if Conf, err = goconf.LoadConfig[Config]("config.toml", goconf.WithIndentSpaces(4), goconf.WithNewFileBehavior(goconf.NewFileBehaviorCreateAndTry)); err != nil {
		err = fmt.Errorf("failed to load config: %w", err)
	}

	return
}
