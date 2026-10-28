package db

import (
	"fmt"

	"github.com/z46-dev/gosqlite"
	"github.com/z46-dev/project-hawaii/config"
)

var (
	driver           *gosqlite.Driver
	Accounts         *gosqlite.RegisteredStruct[Account]
	Identities       *gosqlite.RegisteredStruct[AccountIdentity]
	LocalCredentials *gosqlite.RegisteredStruct[LocalCredential]
	PasswordResets   *gosqlite.RegisteredStruct[PasswordReset]
)

// registerAndMigrate registers a database type and applies its migrations.
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

// Init opens the database and registers all persisted types.
func Init(allowDestructiveMigrations bool) (err error) {
	if driver, err = gosqlite.Begin(config.Conf.Database.FilePath); err != nil {
		err = fmt.Errorf("failed to initialize database driver: %w", err)
		return
	}

	var migrationOpts gosqlite.MigrationOptions = gosqlite.MigrationOptions{
		AllowDestructive: allowDestructiveMigrations,
	}

	if Accounts, err = registerAndMigrate[Account](migrationOpts); err != nil {
		err = fmt.Errorf("failed to register and migrate Account struct: %w", err)
		return
	}

	if Identities, err = registerAndMigrate[AccountIdentity](migrationOpts); err != nil {
		err = fmt.Errorf("failed to register and migrate AccountIdentity struct: %w", err)
		return
	}

	if LocalCredentials, err = registerAndMigrate[LocalCredential](migrationOpts); err != nil {
		err = fmt.Errorf("failed to register and migrate LocalCredential struct: %w", err)
		return
	}

	if PasswordResets, err = registerAndMigrate[PasswordReset](migrationOpts); err != nil {
		err = fmt.Errorf("failed to register and migrate PasswordReset struct: %w", err)
	}

	return
}
