package db

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/z46-dev/gosqlite"
	"golang.org/x/crypto/bcrypt"
)

// CreatePasswordReset replaces a local password with one expiring reset token.
func CreatePasswordReset(accountID int) (token string, err error) {
	var (
		bytes   []byte = make([]byte, 32)
		account *Account
	)

	if _, err = rand.Read(bytes); err != nil {
		return
	}

	token = base64.RawURLEncoding.EncodeToString(bytes)
	if account, err = Accounts.Select(accountID); err != nil {
		return
	}

	account.SessionVersion++
	if err = Accounts.Update(account); err != nil {
		err = fmt.Errorf("invalidate account sessions: %w", err)
		return
	}

	if _, err = LocalCredentials.DeleteWithFilter(gosqlite.NewFilter().KeyCmp(LocalCredentials.FieldByGoName("AccountID"), gosqlite.OpEqual, accountID)); err != nil {
		err = fmt.Errorf("remove local credential: %w", err)
		return
	}

	if _, err = PasswordResets.DeleteWithFilter(gosqlite.NewFilter().KeyCmp(PasswordResets.FieldByGoName("AccountID"), gosqlite.OpEqual, accountID)); err != nil {
		err = fmt.Errorf("replace password reset: %w", err)
		return
	}

	var digest [32]byte = sha256.Sum256([]byte(token))
	err = PasswordResets.Insert(&PasswordReset{AccountID: accountID, TokenHash: base64.RawURLEncoding.EncodeToString(digest[:]), ExpiresAt: time.Now().Add(24 * time.Hour).UTC()})
	return
}

// ConsumePasswordReset validates a token once and stores the new local password.
func ConsumePasswordReset(token, password string) (err error) {
	var (
		digest [32]byte = sha256.Sum256([]byte(token))
		resets []*PasswordReset
		hash   []byte
	)

	if resets, err = PasswordResets.SelectAllWithFilter(gosqlite.NewFilter().KeyCmp(PasswordResets.FieldByGoName("TokenHash"), gosqlite.OpEqual, base64.RawURLEncoding.EncodeToString(digest[:]))); err != nil || len(resets) != 1 || resets[0].ExpiresAt.Before(time.Now()) {
		err = fmt.Errorf("invalid or expired password reset")
		return
	}

	if hash, err = bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost); err != nil {
		return
	}

	if _, err = LocalCredentials.DeleteWithFilter(gosqlite.NewFilter().KeyCmp(LocalCredentials.FieldByGoName("AccountID"), gosqlite.OpEqual, resets[0].AccountID)); err != nil {
		return
	}

	if err = LocalCredentials.Insert(&LocalCredential{AccountID: resets[0].AccountID, PasswordHash: string(hash)}); err != nil {
		return
	}

	_, err = PasswordResets.DeleteWithFilter(gosqlite.NewFilter().KeyCmp(PasswordResets.FieldByGoName("ID"), gosqlite.OpEqual, resets[0].ID))
	return
}
