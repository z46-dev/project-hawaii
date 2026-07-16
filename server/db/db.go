package db

import (
	"fmt"

	"github.com/z46-dev/gosqlite"
	"github.com/z46-dev/project-hawaii/config"
)

var (
	driver   *gosqlite.Driver
	Accounts *gosqlite.RegisteredStruct[Account]
)

func registerAndMigrate[T any]() (registered *gosqlite.RegisteredStruct[T], err error) {
	if registered, err = gosqlite.Register(driver, *new(T)); err != nil {
		err = fmt.Errorf("failed to register struct: %w", err)
		return
	}

	return
}

func Init() (err error) {
	if driver, err = gosqlite.Begin(config.Conf.Database.FilePath); err != nil {
		err = fmt.Errorf("failed to initialize database driver: %w", err)
		return
	}

	return
}
