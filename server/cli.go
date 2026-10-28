package main

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/z46-dev/gosqlite"
	"github.com/z46-dev/project-hawaii/config"
	"github.com/z46-dev/project-hawaii/db"
	"github.com/z46-dev/project-hawaii/pkg/scraper"
)

// createAccount creates a local account and optionally issues its initial password reset.
func createAccount(args CliArguments) {
	var (
		account *db.Account = &db.Account{
			DisplayName:  args.CreateAccount.Username,
			CreationTime: time.Now().UTC(),
			LastActivity: time.Now().UTC(),
			Permissions:  db.AccountPermissionLogin,
		}
		count int64
		err   error
	)

	if count, err = db.Accounts.Count(); err != nil {
		log.Errorf("Failed to count accounts: %+v\n", err)
		return
	}

	if count == 0 {
		account.Permissions |= db.AccountPermissionManageUsers | db.AccountPermissionManageSettings | db.AccountPermissionManageContent
	}

	if err = db.Accounts.Insert(account); err != nil {
		log.Errorf("Failed to create account: %+v\n", err)
		return
	}

	if err = db.Identities.Insert(&db.AccountIdentity{AccountID: account.ID, RealmID: args.CreateAccount.RealmID, Subject: args.CreateAccount.Username, Key: fmt.Sprintf("%s\x00%s", args.CreateAccount.RealmID, args.CreateAccount.Username)}); err != nil {
		log.Errorf("Failed to create account identity: %+v\n", err)
		return
	}

	log.Infof("Created account %q with ID %d\n", account.DisplayName, account.ID)

	if args.CreateAccount.CreatePasswordReset {
		createPasswordResetForAccount(account.ID, args.CreateAccount.PasswordExpirationHours)
	}
}

// createPasswordReset creates a password reset for the specified account identity.
func createPasswordReset(args CliArguments) {
	var (
		identities []*db.AccountIdentity
		err        error
	)

	if identities, err = db.Identities.SelectAllWithFilter(gosqlite.NewFilter().KeyCmp(db.Identities.FieldByGoName("Key"), gosqlite.OpEqual, fmt.Sprintf("%s\x00%s", args.CreatePasswordReset.RealmID, args.CreatePasswordReset.Username))); err != nil {
		log.Errorf("Failed to find account identity: %+v\n", err)
		return
	}

	if len(identities) != 1 {
		log.Errorf("Account %q was not found in realm %q\n", args.CreatePasswordReset.Username, args.CreatePasswordReset.RealmID)
		return
	}

	createPasswordResetForAccount(identities[0].AccountID, args.CreatePasswordReset.PasswordExpirationHours)
}

// createPasswordResetForAccount creates and reports a reset link with the requested expiry.
func createPasswordResetForAccount(accountID, expirationHours int) {
	var (
		token  string
		resets []*db.PasswordReset
		err    error
	)

	if expirationHours <= 0 {
		log.Errorf("Password reset expiration must be positive\n")
		return
	}

	if token, err = db.CreatePasswordReset(accountID); err != nil {
		log.Errorf("Failed to create password reset: %+v\n", err)
		return
	}

	if resets, err = db.PasswordResets.SelectAllWithFilter(gosqlite.NewFilter().KeyCmp(db.PasswordResets.FieldByGoName("AccountID"), gosqlite.OpEqual, accountID)); err != nil || len(resets) != 1 {
		log.Errorf("Failed to find password reset: %+v\n", err)
		return
	}

	resets[0].ExpiresAt = time.Now().Add(time.Duration(expirationHours) * time.Hour).UTC()
	if err = db.PasswordResets.Update(resets[0]); err != nil {
		log.Errorf("Failed to set password reset expiration: %+v\n", err)
		return
	}

	log.Infof("Password reset link: %s/reset-password?token=%s\n", config.Conf.Authentication.FrontendURL, url.QueryEscape(token))
}

func runScraper(args CliArguments) {
	var (
		response *scraper.APISearchResponse
		err      error
	)

	if response, err = scraper.QueryRecentAIReplays(); err != nil {
		log.Errorf("Failed to query recent AI replays: %+v\n", err)
		return
	}

	log.Infof("Found %d replays\n", len(response.Replays))

	if args.Scraper.DownloadReplaysTo != "" {
		// Ensure directory
		if err = os.MkdirAll(args.Scraper.DownloadReplaysTo, 0755); err != nil {
			log.Errorf("Failed to create directory %q: %+v\n", args.Scraper.DownloadReplaysTo, err)
			return
		}
	}

	for _, replay := range response.Replays {
		if args.Scraper.Short {
			log.Infof("Replay ID: %d, Player: %s, Ship: %s, Result: %s, Damage: %d, Frags: %d\n", replay.ID, replay.PlayerName, replay.Ship.Name, replay.Result.Name, replay.Stats.Damage, replay.Stats.Frags)
		} else {
			log.Infof("Replay ID: %d, Player: %s, Ship: %s, Result: %s, Damage: %d, Frags: %d, Map: %s, Mode: %s, Version: %s\n", replay.ID, replay.PlayerName, replay.Ship.Name, replay.Result.Name, replay.Stats.Damage, replay.Stats.Frags, replay.Scenario.MapName, replay.Scenario.ModeName, replay.Version)
		}

		if args.Scraper.DownloadReplaysTo != "" {
			var replayFile *scraper.WowsReplayFile
			if replayFile, err = scraper.DownloadReplay(replay.ID); err != nil {
				log.Errorf("Failed to download replay ID %d: %+v\n", replay.ID, err)
				continue
			}

			if err = replayFile.SaveTo(fmt.Sprintf("%s/replay_%d.wowsreplay", args.Scraper.DownloadReplaysTo, replay.ID)); err != nil {
				log.Errorf("Failed to save replay ID %d: %+v\n", replay.ID, err)
				continue
			}
		}
	}
}
