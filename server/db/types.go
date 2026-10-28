package db

import "time"

type (
	AccountPermissions uint32

	Account struct {
		ID             int                `gosqlite:"id,primary,increment,unique"`
		DisplayName    string             `gosqlite:"display_name"`
		CreationTime   time.Time          `gosqlite:"creation_time"`
		LastActivity   time.Time          `gosqlite:"last_activity"`
		Permissions    AccountPermissions `gosqlite:"permissions"`
		SessionVersion int                `gosqlite:"session_version"`
	}

	AccountIdentity struct {
		ID        int    `gosqlite:"id,primary,increment,unique"`
		AccountID int    `gosqlite:"account_id"`
		RealmID   string `gosqlite:"realm_id"`
		Subject   string `gosqlite:"subject"`
		Key       string `gosqlite:"key,unique"`
	}

	LocalCredential struct {
		ID           int    `gosqlite:"id,primary,increment,unique"`
		AccountID    int    `gosqlite:"account_id,unique"`
		PasswordHash string `gosqlite:"password_hash"`
	}

	PasswordReset struct {
		ID        int       `gosqlite:"id,primary,increment,unique"`
		AccountID int       `gosqlite:"account_id,unique"`
		TokenHash string    `gosqlite:"token_hash,unique"`
		ExpiresAt time.Time `gosqlite:"expires_at"`
	}
)

const (
	AccountPermissionLogin AccountPermissions = 1 << iota
	AccountPermissionManageUsers
	AccountPermissionManageSettings
	AccountPermissionManageContent
)
