package db

import "time"

type (
	Account struct {
		ID           int       `gosqlite:"id,primary,increment,unique"`
		Username     string    `gosqlite:"username,unique"`
		PasswordHash string    `gosqlite:"password_hash"`
		CreationTime time.Time `gosqlite:"creation_time"`
		LastActivity time.Time `gosqlite:"last_activity"`
	}
)
