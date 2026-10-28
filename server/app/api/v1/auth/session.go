package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/project-hawaii/config"
	"github.com/z46-dev/project-hawaii/db"
)

// setSessionCookie stores a signed one-week browser session.
func setSessionCookie(ctx fiber.Ctx, account *db.Account) (err error) {
	var value string
	if value, err = signSession(sessionPayload{AccountID: account.ID, Name: account.DisplayName, Version: account.SessionVersion, ExpiresAt: time.Now().Add(7 * 24 * time.Hour).Unix()}); err != nil {
		return
	}

	ctx.Cookie(&fiber.Cookie{Name: authSessionCookie, Value: value, Path: "/", MaxAge: 604800, HTTPOnly: true, SameSite: "Lax", Secure: usesHTTPS()})
	return
}

// signSession signs serialized session data.
func signSession(payload sessionPayload) (value string, err error) {
	var (
		encoded []byte
		mac     hash.Hash
	)

	if encoded, err = json.Marshal(payload); err != nil {
		return
	}

	value = base64.RawURLEncoding.EncodeToString(encoded)
	mac = hmac.New(sha256.New, []byte(config.Conf.Authentication.SessionSecret))
	mac.Write([]byte(value))
	value += "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return
}

// readSession verifies signed session data.
func readSession(value string) (payload sessionPayload, err error) {
	var (
		parts              []string = strings.Split(value, ".")
		signature, encoded []byte
		mac                hash.Hash
	)

	if len(parts) != 2 || config.Conf.Authentication.SessionSecret == "" {
		err = fmt.Errorf("invalid session")
		return
	}

	if signature, err = base64.RawURLEncoding.DecodeString(parts[1]); err != nil {
		return
	}

	mac = hmac.New(sha256.New, []byte(config.Conf.Authentication.SessionSecret))
	mac.Write([]byte(parts[0]))

	if !hmac.Equal(signature, mac.Sum(nil)) {
		err = fmt.Errorf("invalid session signature")
		return
	}

	if encoded, err = base64.RawURLEncoding.DecodeString(parts[0]); err != nil {
		return
	}

	if err = json.Unmarshal(encoded, &payload); err != nil {
		return
	}

	if payload.ExpiresAt <= time.Now().Unix() {
		err = fmt.Errorf("session expired")
	}

	return
}

func redirectToFrontend(ctx fiber.Ctx, result string) (err error) {
	err = ctx.Redirect().Status(fiber.StatusFound).To(config.Conf.Authentication.FrontendURL + "/?auth=" + url.QueryEscape(result))
	return
}

func usesHTTPS() (uses bool) {
	uses = strings.HasPrefix(config.Conf.Authentication.FrontendURL, "https://")
	return
}

func randomToken() (token string, err error) {
	var bytes []byte = make([]byte, 32)
	if _, err = rand.Read(bytes); err != nil {
		return
	}

	token = base64.RawURLEncoding.EncodeToString(bytes)
	return
}
