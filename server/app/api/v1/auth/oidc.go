package auth

import (
	"context"
	"crypto/hmac"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/project-hawaii/config"
	"github.com/z46-dev/project-hawaii/db"
	"golang.org/x/oauth2"
)

// OIDCLogin starts the authorization-code flow for an OIDC realm.
func OIDCLogin(ctx fiber.Ctx) (err error) {
	var (
		realm    *config.Realm
		provider *oidc.Provider
		oauth    oauth2.Config
		state    string
	)

	if realm, err = realmByID(ctx.Params("realm")); err != nil || realm.Type != "oidc" {
		err = fiber.NewError(fiber.StatusNotFound, "OIDC realm not found")
		return
	}

	if provider, err = oidc.NewProvider(context.Background(), realm.Settings["issuer"]); err != nil {
		err = fmt.Errorf("discover OIDC provider: %w", err)
		return
	}

	if state, err = randomToken(); err != nil {
		err = fmt.Errorf("generate state: %w", err)
		return
	}

	ctx.Cookie(&fiber.Cookie{Name: authStateCookie, Value: realm.ID + ":" + state, Path: "/api/v1/auth/oidc", MaxAge: 600, HTTPOnly: true, SameSite: "Lax", Secure: usesHTTPS()})
	oauth = oauthConfig(*realm, provider)
	err = ctx.Redirect().Status(fiber.StatusFound).To(oauth.AuthCodeURL(state, oauth2.AccessTypeOnline))
	return
}

// OIDCCallback verifies the provider identity and establishes a browser session.
func OIDCCallback(ctx fiber.Ctx) (err error) {
	var (
		realm    *config.Realm
		provider *oidc.Provider
		oauth    oauth2.Config
		token    *oauth2.Token
		idToken  *oidc.IDToken
		claims   oidcClaims
		account  *db.Account
		state    string = ctx.Query("state")
	)

	if realm, err = realmByID(ctx.Params("realm")); err != nil || realm.Type != "oidc" || state == "" || !hmac.Equal([]byte(realm.ID+":"+state), []byte(ctx.Cookies(authStateCookie))) || ctx.Query("code") == "" {
		err = redirectToFrontend(ctx, "error")
		return
	}

	ctx.ClearCookie(authStateCookie)
	if provider, err = oidc.NewProvider(context.Background(), realm.Settings["issuer"]); err != nil {
		err = redirectToFrontend(ctx, "error")
		return
	}

	oauth = oauthConfig(*realm, provider)
	if token, err = oauth.Exchange(context.Background(), ctx.Query("code")); err != nil {
		err = redirectToFrontend(ctx, "error")
		return
	}

	if rawIDToken, ok := token.Extra("id_token").(string); ok {
		if idToken, err = provider.Verifier(&oidc.Config{ClientID: realm.Settings["client_id"]}).Verify(context.Background(), rawIDToken); err != nil {
			err = redirectToFrontend(ctx, "error")
			return
		}
	} else {
		err = redirectToFrontend(ctx, "error")
		return
	}

	if err = idToken.Claims(&claims); err != nil {
		err = redirectToFrontend(ctx, "error")
		return
	}

	if claims.Subject == "" {
		err = redirectToFrontend(ctx, "error")
		return
	}

	if account, err = accountForIdentity(*realm, claims.Subject, firstValue(claims.Name, claims.PreferredUsername, claims.Subject)); err != nil {
		err = redirectToFrontend(ctx, "error")
		return
	}

	if err = setSessionCookie(ctx, account); err != nil {
		err = fmt.Errorf("set session: %w", err)
		return
	}

	err = redirectToFrontend(ctx, "success")
	return
}

func oauthConfig(realm config.Realm, provider *oidc.Provider) oauth2.Config {
	return oauth2.Config{ClientID: realm.Settings["client_id"], ClientSecret: realm.Settings["client_secret"], Endpoint: provider.Endpoint(), RedirectURL: realm.Settings["callback_url"], Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}
}
