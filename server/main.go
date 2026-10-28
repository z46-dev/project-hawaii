package main

import (
	"github.com/alexflint/go-arg"
	"github.com/z46-dev/golog"
	"github.com/z46-dev/project-hawaii/app"
	"github.com/z46-dev/project-hawaii/config"
	"github.com/z46-dev/project-hawaii/db"
)

type (
	AccountOperationalArguments struct {
		RealmID                 string `arg:"positional"`
		Username                string `arg:"positional"`
		PasswordExpirationHours int    `arg:"--password-expiration-hours" default:"3"`
	}

	CreatePasswordResetArguments struct {
		AccountOperationalArguments
	}

	CreateAccountArguments struct {
		AccountOperationalArguments
		CreatePasswordReset bool `arg:"--create-password-reset"`
	}

	CliArguments struct {
		AllowDestructiveMigrations bool                          `arg:"--allow-destructive-migrations"`
		CreatePasswordReset        *CreatePasswordResetArguments `arg:"subcommand:password_reset"`
		CreateAccount              *CreateAccountArguments       `arg:"subcommand:create_account"`
	}
)

var log *golog.Logger = golog.New().Prefix("[HAWAII]", golog.BoldCyan).Timestamp()

func main() {
	var (
		args CliArguments
		err  error
	)

	arg.MustParse(&args)

	if err = config.Init(); err != nil {
		log.Errorf("Failed to init the config: %+v\n", err)
		return
	}

	if err = db.Init(args.AllowDestructiveMigrations); err != nil {
		log.Errorf("Failed to init the database: %+v\n", err)
		return
	}

	switch {
	case args.CreateAccount != nil:
		createAccount(args)
	case args.CreatePasswordReset != nil:
		createPasswordReset(args)
	default:
		if err = app.Init(log); err != nil {
			log.Errorf("Failed to init the app: %+v\n", err)
			return
		}
	}
}
