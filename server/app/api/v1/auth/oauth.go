package auth

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/z46-dev/project-hawaii/config"
	"github.com/z46-dev/project-hawaii/db"
	"golang.org/x/oauth2"
)

// OAuthLogin starts the authorization-code flow for a generic OAuth2 realm.
func OAuthLogin(ctx fiber.Ctx) (err error) {
	var (
		realm *config.Realm
		state string
		oauth oauth2.Config
	)

	if realm, err = realmByID(ctx.Params("realm")); err != nil || realm.Type != "oauth" {
		err = fiber.NewError(fiber.StatusNotFound, "OAuth realm not found")
		return
	}

	if state, err = randomToken(); err != nil {
		err = fmt.Errorf("generate state: %w", err)
		return
	}

	ctx.Cookie(&fiber.Cookie{Name: authStateCookie, Value: realm.ID + ":" + state, Path: "/api/v1/auth/oauth", MaxAge: 600, HTTPOnly: true, SameSite: "Lax", Secure: usesHTTPS()})
	oauth = genericOAuthConfig(*realm)
	err = ctx.Redirect().Status(fiber.StatusFound).To(oauth.AuthCodeURL(state, oauth2.AccessTypeOnline))
	return
}

// OAuthCallback maps a generic OAuth2 user-info response to a realm identity.
func OAuthCallback(ctx fiber.Ctx) (err error) {
	var (
		realm   *config.Realm
		token   *oauth2.Token
		oauth   oauth2.Config
		account *db.Account
		profile map[string]any
		state   string = ctx.Query("state")
	)

	if realm, err = realmByID(ctx.Params("realm")); err != nil || realm.Type != "oauth" || state == "" || !hmac.Equal([]byte(realm.ID+":"+state), []byte(ctx.Cookies(authStateCookie))) || ctx.Query("code") == "" {
		err = redirectToFrontend(ctx, "error")
		return
	}

	ctx.ClearCookie(authStateCookie)
	oauth = genericOAuthConfig(*realm)
	if token, err = oauth.Exchange(context.Background(), ctx.Query("code")); err != nil {
		err = redirectToFrontend(ctx, "error")
		return
	}

	if profile, err = fetchOAuthProfile(*realm, token.AccessToken); err != nil {
		err = redirectToFrontend(ctx, "error")
		return
	}

	if account, err = accountForIdentity(*realm, stringValue(profile[realmSubjectField(*realm)]), stringValue(profile[realmNameField(*realm)])); err != nil {
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

// genericOAuthConfig builds an authorization-code client from a realm's OAuth2 settings.
func genericOAuthConfig(realm config.Realm) (oauth oauth2.Config) {
	oauth = oauth2.Config{ClientID: realm.Settings["client_id"], ClientSecret: realm.Settings["client_secret"], Endpoint: oauth2.Endpoint{AuthURL: realm.Settings["authorization_url"], TokenURL: realm.Settings["token_url"]}, RedirectURL: realm.Settings["callback_url"], Scopes: strings.Fields(realm.Settings["scopes"])}
	return
}

// fetchOAuthProfile retrieves the configured provider's user-info document.
func fetchOAuthProfile(realm config.Realm, accessToken string) (profile map[string]any, err error) {
	var (
		request  *http.Request
		response *http.Response
		body     []byte
	)

	if request, err = http.NewRequest(http.MethodGet, realm.Settings["user_info_url"], nil); err != nil {
		return
	}

	request.Header.Set("Authorization", "Bearer "+accessToken)
	if response, err = http.DefaultClient.Do(request); err != nil {
		return
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		err = fmt.Errorf("OAuth user-info returned %s", response.Status)
		return
	}

	if body, err = io.ReadAll(response.Body); err != nil {
		return
	}

	if err = json.Unmarshal(body, &profile); err != nil {
		err = fmt.Errorf("decode OAuth user-info: %w", err)
	}

	return
}

func realmSubjectField(realm config.Realm) (field string) {
	field = firstValue(realm.Settings["subject_field"], "id")
	return
}

func realmNameField(realm config.Realm) (field string) {
	field = firstValue(realm.Settings["name_field"], "username")
	return
}

func stringValue(value any) (str string) {
	if stringValue, ok := value.(string); ok {
		str = stringValue
	}

	return
}
