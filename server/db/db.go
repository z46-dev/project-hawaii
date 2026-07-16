package db

import (
	"fmt"
	"os"
	"slices"

	"github.com/z46-dev/gosqlite"
	"github.com/z46-dev/project-hawaii/config"
)

var (
	driver   *gosqlite.Driver
	Accounts *gosqlite.RegisteredStruct[Account]
)

func registerAndMigrate[T any](opts gosqlite.MigrationOptions) (registered *gosqlite.RegisteredStruct[T], err error) {
	if registered, err = gosqlite.Register(driver, *new(T)); err != nil {
		err = fmt.Errorf("failed to register struct: %w", err)
		return
	}

	var migrationReport *gosqlite.MigrationReport
	if migrationReport, err = registered.Migrate(opts); err != nil {
		err = fmt.Errorf("failed to migrate struct: %w", err)
		return
	}

	if migrationReport != nil && migrationReport.Rebuilt {
		fmt.Printf("Migration report indicates that the struct was rebuilt: %+v\n", migrationReport)
	}

	return
}

func Init() (err error) {
	if driver, err = gosqlite.Begin(config.Conf.Database.FilePath); err != nil {
		err = fmt.Errorf("failed to initialize database driver: %w", err)
		return
	}

	var migrationOpts gosqlite.MigrationOptions = gosqlite.MigrationOptions{
		AllowDestructive: slices.Contains(os.Args, "--allow-destructive-migrations"),
	}

	if Accounts, err = registerAndMigrate[Account](migrationOpts); err != nil {
		err = fmt.Errorf("failed to register and migrate Account struct: %w", err)
		return
	}

	return
}
