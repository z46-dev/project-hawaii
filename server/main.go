package main

import (
	"github.com/z46-dev/golog"
	"github.com/z46-dev/project-hawaii/app"
	"github.com/z46-dev/project-hawaii/config"
	"github.com/z46-dev/project-hawaii/db"
)

var (
	log *golog.Logger = golog.New().Prefix("[HAWAII]", golog.BoldCyan).Timestamp()
)

func main() {
	var err error

	if err = config.Init(); err != nil {
		log.Errorf("Failed to init the config: %+v\n", err)
		return
	}

	if err = db.Init(); err != nil {
		log.Errorf("Failed to init the database: %+v\n", err)
		return
	}

	if err = app.Init(log); err != nil {
		log.Errorf("Failed to init the app: %+v\n", err)
		return
	}
}
