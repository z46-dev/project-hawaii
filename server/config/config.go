package config

import (
	"fmt"

	"github.com/z46-dev/goconf"
)

type Config struct {
	WebServer struct {
		BindAddress     string   `toml:"bind_address" default:"[::]:1944" validate:"required"`
		AcceptedOrigins []string `toml:"accepted_origins" default:"[\"http://localhost:3000\"]" validate:"required"`
	} `toml:"web_server"`
	Database struct {
		FilePath string `toml:"file_path" default:"./hawaii-sqlite.db" validate:"required"`
	} `toml:"database"`
	LLMServer struct {
		OpenAI_Like_API_URL string `toml:"openai_like_api_url" default:"http://localhost:1234/v1" validate:"required"`
		OpenAI_Like_API_Key string `toml:"openai_like_api_key" default:"your_key" validate:"required"`
	} `toml:"llm_server"`
}

var Conf Config

func Init() (err error) {
	if Conf, err = goconf.LoadConfig[Config]("config.toml", goconf.WithIndentSpaces(4), goconf.WithNewFileBehavior(goconf.NewFileBehaviorCreateAndTry)); err != nil {
		err = fmt.Errorf("failed to load config: %w", err)
	}

	return
}
