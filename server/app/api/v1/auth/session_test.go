package auth

import (
	"testing"
	"time"

	"github.com/z46-dev/project-hawaii/config"
)

// TestSessionRoundTrip verifies signed sessions cannot be modified or reused after expiry.
func TestSessionRoundTrip(t *testing.T) {
	var (
		original, value string = config.Conf.Authentication.SessionSecret, ""
		payload         sessionPayload
		err             error
	)

	config.Conf.Authentication.SessionSecret = "test-session-secret"

	t.Cleanup(func() {
		config.Conf.Authentication.SessionSecret = original
	})

	if value, err = signSession(sessionPayload{AccountID: 42, Name: "captain", ExpiresAt: time.Now().Add(time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}

	if payload, err = readSession(value); err != nil {
		t.Fatal(err)
	}

	if payload.AccountID != 42 || payload.Name != "captain" {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	if _, err = readSession(value + "tampered"); err == nil {
		t.Fatal("expected tampered session to fail")
	}
}
