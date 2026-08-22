package store_test

import (
	"errors"
	"strings"
	"testing"

	"queueup/internal/store"
)

func TestRegisterAndSignIn(t *testing.T) {
	s := newStore(t)

	acct, err := s.Register("Player@Example.com ", "correct horse battery")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if acct.Email != "player@example.com" {
		t.Errorf("email = %q; it should be trimmed and lowercased", acct.Email)
	}

	got, token, err := s.SignIn("player@example.com", "correct horse battery")
	if err != nil || got.ID != acct.ID || token == "" {
		t.Fatalf("SignIn = %v, %q, %v", got, token, err)
	}

	// The session token works...
	session, err := s.AccountBySession(token)
	if err != nil || session.ID != acct.ID {
		t.Fatalf("AccountBySession = %v, %v", session, err)
	}
	// ...until it is signed out.
	if err := s.SignOut(token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AccountBySession(token); err == nil {
		t.Fatal("a signed-out session still works")
	}
}

func TestWrongPasswordAndUnknownEmailLookIdentical(t *testing.T) {
	s := newStore(t)
	if _, err := s.Register("player@example.com", "correct horse battery"); err != nil {
		t.Fatal(err)
	}

	_, _, wrongPass := s.SignIn("player@example.com", "not the password")
	_, _, noSuchUser := s.SignIn("nobody@example.com", "not the password")

	if !errors.Is(wrongPass, store.ErrBadCredentials) || !errors.Is(noSuchUser, store.ErrBadCredentials) {
		t.Fatalf("errors = %v and %v; both should be ErrBadCredentials", wrongPass, noSuchUser)
	}
	// Same wording, so the sign-in page cannot be used to discover which email
	// addresses have accounts.
	if wrongPass.Error() != noSuchUser.Error() {
		t.Errorf("the two failures read differently: %q vs %q", wrongPass, noSuchUser)
	}
}

func TestPasswordIsNeverStoredInTheClear(t *testing.T) {
	s := newStore(t)
	const password = "correct horse battery"
	if _, err := s.Register("player@example.com", password); err != nil {
		t.Fatal(err)
	}
	// Nothing anywhere in the database should contain the password itself.
	dump, err := s.DebugDump()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dump, password) {
		t.Fatal("the password appears in the database in readable form")
	}
}

func TestRegistrationRefusesWeakInput(t *testing.T) {
	s := newStore(t)
	for _, c := range []struct{ email, password, why string }{
		{"not-an-email", "long enough password", "no @ sign"},
		{"a@b.com", "short", "password under 8 characters"},
	} {
		if _, err := s.Register(c.email, c.password); err == nil {
			t.Errorf("accepted %s", c.why)
		} else if len(err.Error()) < 15 {
			t.Errorf("error for %s is too terse to show a user: %q", c.why, err)
		}
	}
	if _, err := s.Register("dupe@example.com", "long enough password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register("dupe@example.com", "long enough password"); err == nil {
		t.Error("the same email was registered twice")
	}
}

func TestFavourites(t *testing.T) {
	s := newStore(t)
	acct, _ := newAccount(t, s)

	if favs, err := s.Favourites(acct.ID); err != nil || len(favs) != 0 {
		t.Fatalf("a new account has %d saved servers, want 0 (%v)", len(favs), err)
	}

	f := store.Favourite{ServerID: "stub-1", Name: "Rustopia EU Main", Address: "51.83.128.10:28015"}
	if err := s.AddFavourite(acct.ID, f); err != nil {
		t.Fatal(err)
	}
	// Saving the same server twice updates it rather than duplicating it.
	f.Name = "Rustopia EU Main (renamed)"
	if err := s.AddFavourite(acct.ID, f); err != nil {
		t.Fatal(err)
	}
	favs, err := s.Favourites(acct.ID)
	if err != nil || len(favs) != 1 {
		t.Fatalf("saved servers = %d, want 1 (%v)", len(favs), err)
	}
	if favs[0].Name != "Rustopia EU Main (renamed)" {
		t.Errorf("name = %q, want the updated one", favs[0].Name)
	}

	if err := s.RemoveFavourite(acct.ID, "stub-1"); err != nil {
		t.Fatal(err)
	}
	if favs, _ := s.Favourites(acct.ID); len(favs) != 0 {
		t.Fatalf("saved servers after removing = %d, want 0", len(favs))
	}
}
