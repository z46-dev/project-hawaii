package auth

const (
	authStateCookie   string = "auth_realm_state"
	authSessionCookie string = "project_hawaii_session"
)

type (
	credentials struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	passwordReset struct {
		Password string `json:"password"`
	}

	sessionPayload struct {
		AccountID int    `json:"account_id"`
		Name      string `json:"name"`
		Version   int    `json:"version"`
		ExpiresAt int64  `json:"expires_at"`
	}

	oidcClaims struct {
		Subject           string `json:"sub"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}
)
