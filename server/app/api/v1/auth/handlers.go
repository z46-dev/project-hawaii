package auth

import (
	"fmt"
	"net/url"

	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/project-hawaii/config"
	"github.com/z46-dev/project-hawaii/db"
	"golang.org/x/crypto/bcrypt"
)

// Realms lists the configured authentication choices without exposing their settings.
func Realms(ctx fiber.Ctx) (err error) {
	var realms []fiber.Map = make([]fiber.Map, 0, len(config.Conf.Authentication.Realms))

	for _, realm := range config.Conf.Authentication.Realms {
		if realm.ID != "" && realm.Name != "" && realm.Type != "" {
			realms = append(realms, fiber.Map{
				"id":                 realm.ID,
				"name":               realm.Name,
				"type":               realm.Type,
				"allow_registration": realm.Type == "local" && realm.Settings["allow_registration"] == "true",
			})
		}
	}

	err = ctx.JSON(fiber.Map{"realms": realms})
	return
}

// Login authenticates local-password and LDAP realms.
func Login(ctx fiber.Ctx) (err error) {
	var (
		realm   *config.Realm
		input   credentials
		account *db.Account
	)

	if realm, err = realmByID(ctx.Params("realm")); err != nil {
		err = fiber.NewError(fiber.StatusNotFound, "authentication realm not found")
		return
	}

	switch realm.Type {
	case "local", "ldap":
	default:
		err = fiber.NewError(fiber.StatusBadRequest, "this realm uses browser sign-in")
		return
	}

	if err = ctx.Bind().JSON(&input); err != nil {
		err = fiber.NewError(fiber.StatusBadRequest, "invalid credentials")
		return
	}

	if realm.Type == "local" {
		account, err = localAccount(*realm, input)
	} else {
		account, err = ldapAccount(*realm, input)
	}

	if err != nil {
		err = fiber.NewError(fiber.StatusUnauthorized, "invalid username or password")
		return
	}

	if err = setSessionCookie(ctx, account); err != nil {
		err = fmt.Errorf("set session: %w", err)
		return
	}

	err = ctx.JSON(fiber.Map{"authenticated": true, "user": fiber.Map{"id": account.ID, "name": account.DisplayName}})
	return
}

// RegisterLocal creates a local account when the selected realm allows self-registration.
func RegisterLocal(ctx fiber.Ctx) (err error) {
	var (
		realm   *config.Realm
		input   credentials
		account *db.Account
		hash    []byte
	)

	if realm, err = realmByID(ctx.Params("realm")); err != nil || realm.Type != "local" {
		err = fiber.NewError(fiber.StatusNotFound, "local realm not found")
		return
	}

	if realm.Settings["allow_registration"] != "true" {
		err = fiber.NewError(fiber.StatusForbidden, "local registration is disabled")
		return
	}

	if err = ctx.Bind().JSON(&input); err != nil || input.Username == "" || len(input.Password) < 12 {
		err = fiber.NewError(fiber.StatusBadRequest, "a username and a password of at least 12 characters are required")
		return
	}

	if hash, err = bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost); err != nil {
		err = fmt.Errorf("hash local password: %w", err)
		return
	}

	if account, err = accountForIdentity(*realm, input.Username, input.Username); err != nil {
		err = fiber.NewError(fiber.StatusConflict, "username is already registered")
		return
	}

	if err = db.LocalCredentials.Insert(&db.LocalCredential{AccountID: account.ID, PasswordHash: string(hash)}); err != nil {
		err = fiber.NewError(fiber.StatusConflict, "username is already registered")
		return
	}

	if err = setSessionCookie(ctx, account); err != nil {
		err = fmt.Errorf("set session: %w", err)
		return
	}

	err = ctx.JSON(fiber.Map{"authenticated": true, "user": fiber.Map{"id": account.ID, "name": account.DisplayName}})
	return
}

// Me returns the account represented by the signed session cookie.
func Me(ctx fiber.Ctx) (err error) {
	var (
		payload sessionPayload
		account *db.Account
	)

	if payload, err = readSession(ctx.Cookies(authSessionCookie)); err != nil {
		err = fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		return
	}

	if account, err = db.Accounts.Select(payload.AccountID); err != nil || account.SessionVersion != payload.Version {
		err = fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		return
	}

	err = ctx.JSON(fiber.Map{"authenticated": true, "user": fiber.Map{"id": payload.AccountID, "name": payload.Name}})
	return
}

// Logout removes the signed browser session.
func Logout(ctx fiber.Ctx) (err error) {
	ctx.Cookie(&fiber.Cookie{Name: authSessionCookie, Value: "", Path: "/", MaxAge: -1, HTTPOnly: true, SameSite: "Lax", Secure: usesHTTPS()})
	err = ctx.SendStatus(fiber.StatusNoContent)
	return
}

// ResetPassword consumes a one-time password reset token and returns the user to login.
func ResetPassword(ctx fiber.Ctx) (err error) {
	var input passwordReset

	if err = ctx.Bind().JSON(&input); err != nil || len(input.Password) < 12 {
		err = fiber.NewError(fiber.StatusBadRequest, "a password of at least 12 characters is required")
		return
	}

	if err = db.ConsumePasswordReset(ctx.Params("token"), input.Password); err != nil {
		err = fiber.NewError(fiber.StatusBadRequest, "invalid or expired password reset")
		return
	}

	err = ctx.SendStatus(fiber.StatusNoContent)
	return
}

// CreatePasswordResetLink creates the single-use link consumed by ResetPassword.
func CreatePasswordResetLink(accountID int) (link string, err error) {
	var token string

	if token, err = db.CreatePasswordReset(accountID); err != nil {
		return
	}

	link = config.Conf.Authentication.FrontendURL + "/reset-password?token=" + url.QueryEscape(token)
	return
}
