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
		RealmID                 string `arg:"positional" help:"The realm ID of the account"`
		Username                string `arg:"positional" help:"The username of the account"`
		PasswordExpirationHours int    `arg:"-e,--password-expiration-hours" default:"3" help:"The number of hours before the password expires"`
	}

	CreatePasswordResetArguments struct {
		AccountOperationalArguments
	}

	CreateAccountArguments struct {
		AccountOperationalArguments
		CreatePasswordReset bool `arg:"-p,--create-password-reset" help:"Whether to create a password reset for the account"`
	}

	ScraperArguments struct {
		Short             bool   `arg:"-s,--short" help:"Prune output to player name, ship name, and result (with damage and frags)"`
		Count             int    `arg:"-c,--count" default:"0" help:"The number of replays to scrape, 0 for the most recent page"`
		DownloadReplaysTo string `arg:"-d,--download-replays-to" help:"Download replays to the specified directory"`
		ParallelDownloads uint   `arg:"-p,--parallel-downloads" default:"4" help:"The number of parallel downloads to perform when downloading replays"`
	}

	CliArguments struct {
		AllowDestructiveMigrations bool                          `arg:"--allow-destructive-migrations" help:"Whether to allow destructive migrations on the database"`
		CreatePasswordReset        *CreatePasswordResetArguments `arg:"subcommand:password_reset" help:"Create a password reset for an account"`
		CreateAccount              *CreateAccountArguments       `arg:"subcommand:create_account" help:"Create an account"`
		Scraper                    *ScraperArguments             `arg:"subcommand:scraper" help:"Run the scraper"`
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
	case args.Scraper != nil:
		runScraper(args)
	default:
		if err = app.Init(log); err != nil {
			log.Errorf("Failed to init the app: %+v\n", err)
			return
		}
	}
}
