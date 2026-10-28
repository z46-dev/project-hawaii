package auth

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/z46-dev/gosqlite"
	"github.com/z46-dev/project-hawaii/config"
	"github.com/z46-dev/project-hawaii/db"
	"golang.org/x/crypto/bcrypt"
)

// localAccount verifies a password against the local credential for a realm identity.
func localAccount(realm config.Realm, input credentials) (account *db.Account, err error) {
	var credential *db.LocalCredential

	if account, credential, err = accountAndCredential(realm.ID, input.Username); err != nil {
		return
	}

	if err = bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte(input.Password)); err != nil {
		err = fmt.Errorf("password mismatch")
	}

	return
}

// ldapAccount verifies credentials against LDAP and maps the directory identity to an account.
func ldapAccount(realm config.Realm, input credentials) (account *db.Account, err error) {
	var (
		connection *ldap.Conn
		result     *ldap.SearchResult
		entry      *ldap.Entry
	)

	if connection, err = ldap.DialURL(realm.Settings["url"]); err != nil {
		return
	}

	defer connection.Close()

	if err = connection.Bind(realm.Settings["bind_dn"], realm.Settings["bind_password"]); err != nil {
		return
	}

	if result, err = connection.Search(ldap.NewSearchRequest(realm.Settings["user_base_dn"], ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 0, false, strings.ReplaceAll(firstValue(realm.Settings["user_filter"], "(uid=%s)"), "%s", ldap.EscapeFilter(input.Username)), []string{"dn"}, nil)); err != nil || len(result.Entries) != 1 {
		err = fmt.Errorf("LDAP user not found")
		return
	}

	entry = result.Entries[0]
	if err = connection.Bind(entry.DN, input.Password); err != nil {
		return
	}

	account, err = accountForIdentity(realm, entry.DN, input.Username)
	return
}

// accountAndCredential finds a local realm identity with its credential.
func accountAndCredential(realmID, subject string) (account *db.Account, credential *db.LocalCredential, err error) {
	var identities []*db.AccountIdentity

	if identities, err = db.Identities.SelectAllWithFilter(gosqlite.NewFilter().KeyCmp(db.Identities.FieldByGoName("Key"), gosqlite.OpEqual, identityKey(realmID, subject))); err != nil || len(identities) != 1 {
		err = fmt.Errorf("local account not found")
		return
	}

	if account, err = db.Accounts.Select(identities[0].AccountID); err != nil {
		return
	}

	var credentials []*db.LocalCredential
	if credentials, err = db.LocalCredentials.SelectAllWithFilter(gosqlite.NewFilter().KeyCmp(db.LocalCredentials.FieldByGoName("AccountID"), gosqlite.OpEqual, account.ID)); err != nil || len(credentials) != 1 {
		err = fmt.Errorf("local credential not found")
		return
	}

	credential = credentials[0]
	return
}

// accountForIdentity finds or provisions an account for a realm identity.
func accountForIdentity(realm config.Realm, subject, name string) (account *db.Account, err error) {
	var identities []*db.AccountIdentity

	if identities, err = db.Identities.SelectAllWithFilter(gosqlite.NewFilter().KeyCmp(db.Identities.FieldByGoName("Key"), gosqlite.OpEqual, identityKey(realm.ID, subject))); err != nil {
		return
	}

	if len(identities) > 0 {
		account, err = db.Accounts.Select(identities[0].AccountID)
		return
	}

	account = &db.Account{DisplayName: name, CreationTime: time.Now().UTC(), LastActivity: time.Now().UTC(), Permissions: db.AccountPermissionLogin}
	if err = db.Accounts.Insert(account); err != nil {
		return
	}

	err = db.Identities.Insert(&db.AccountIdentity{AccountID: account.ID, RealmID: realm.ID, Subject: subject, Key: identityKey(realm.ID, subject)})
	return
}

// realmByID resolves a configured realm.
func realmByID(id string) (realm *config.Realm, err error) {
	for index := range config.Conf.Authentication.Realms {
		if config.Conf.Authentication.Realms[index].ID == id {
			realm = &config.Conf.Authentication.Realms[index]
			return
		}
	}

	err = fmt.Errorf("realm not found")
	return
}

func identityKey(realmID, subject string) (key string) {
	key = fmt.Sprintf("%s\x00%s", realmID, subject)
	return
}

func firstValue(values ...string) (first string) {
	for _, value := range values {
		if value != "" {
			first = value
			return
		}
	}

	return
}
