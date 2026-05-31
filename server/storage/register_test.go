package storage

import (
	"sync"
	"testing"
	"time"
)

func TestRegisterWithInvite_Valid(t *testing.T) {
	store := newTestStore(t)

	code := "reg-code-valid"
	err := store.CreateInvite(code, time.Now().Add(1*time.Hour), "admin")
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.RegisterWithInvite(code, "newuser", "password")
	if err != nil {
		t.Fatalf("RegisterWithInvite failed: %v", err)
	}
	if !created {
		t.Error("Expected user to be created")
	}

	// Verify user exists and can log in
	ok, err := store.VerifyUser("newuser", "password")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("User should be able to log in")
	}

	// Verify invite is marked used
	valid, err := store.ValidateInvite(code)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Error("Invite should be marked used")
	}
}

func TestRegisterWithInvite_InvalidCode(t *testing.T) {
	store := newTestStore(t)

	created, err := store.RegisterWithInvite("nonexistent", "newuser", "password")
	if err == nil {
		t.Error("Expected error for invalid invite code")
	}
	if created {
		t.Error("Expected no user creation")
	}
}

func TestRegisterWithInvite_ExpiredCode(t *testing.T) {
	store := newTestStore(t)

	code := "reg-code-expired"
	err := store.CreateInvite(code, time.Now().Add(-1*time.Hour), "admin")
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.RegisterWithInvite(code, "newuser", "password")
	if err == nil {
		t.Error("Expected error for expired invite code")
	}
	if created {
		t.Error("Expected no user creation")
	}
}

func TestRegisterWithInvite_AlreadyUsedCode(t *testing.T) {
	store := newTestStore(t)

	code := "reg-code-used"
	err := store.CreateInvite(code, time.Now().Add(1*time.Hour), "admin")
	if err != nil {
		t.Fatal(err)
	}

	// First use succeeds
	created, err := store.RegisterWithInvite(code, "alice", "password")
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}
	if !created {
		t.Error("First registration should succeed")
	}

	// Second use fails
	created, err = store.RegisterWithInvite(code, "bob", "password")
	if err == nil {
		t.Error("Expected error for already-used invite code")
	}
	if created {
		t.Error("Expected no user creation for second registration")
	}

	// Verify only alice was created
	ok, err := store.VerifyUser("alice", "password")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("Alice should exist")
	}

	ok, err = store.VerifyUser("bob", "password")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("Bob should not exist — registration should have been rolled back")
	}
}

func TestRegisterWithInvite_DuplicateUser(t *testing.T) {
	store := newTestStore(t)

	// Create user first
	err := store.CreateUser("existing", "password")
	if err != nil {
		t.Fatal(err)
	}

	// Try to register same user with an invite
	code := "reg-code-dup"
	err = store.CreateInvite(code, time.Now().Add(1*time.Hour), "admin")
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.RegisterWithInvite(code, "existing", "newpass")
	if err == nil {
		t.Error("Expected error for duplicate user")
	}
	if created {
		t.Error("Expected no user creation")
	}

	// Verify original user still exists with original password
	ok, err := store.VerifyUser("existing", "password")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("Original user should still exist with original password")
	}

	// Verify invite was NOT marked used (transaction rolled back)
	valid, err := store.ValidateInvite(code)
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Error("Invite should still be valid — transaction rolled back on duplicate user")
	}
}

func TestRegisterWithInvite_ConcurrentSameCode_DifferentUsers(t *testing.T) {
	store := newTestStore(t)

	code := "reg-concurrent-diff"
	err := store.CreateInvite(code, time.Now().Add(1*time.Hour), "admin")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make([]bool, 10)
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			username := "user" + string(rune('0'+idx))
			_, err := store.RegisterWithInvite(code, username, "password")
			mu.Lock()
			results[idx] = err == nil
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	// Exactly one registration should succeed
	succeeded := 0
	for _, r := range results {
		if r {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Errorf("Expected exactly 1 successful registration, got %d", succeeded)
	}

	// Verify only one user was created
	count, err := store.CreateUserCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("Expected exactly 1 user created, got %d", count)
	}
}

func TestRegisterWithInvite_ConcurrentSameCode_SameUser(t *testing.T) {
	store := newTestStore(t)

	code := "reg-concurrent-same"
	err := store.CreateInvite(code, time.Now().Add(1*time.Hour), "admin")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make([]bool, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			created, err := store.RegisterWithInvite(code, "sameuser", "password")
			results[idx] = err == nil && created
		}(i)
	}
	wg.Wait()

	// Exactly one registration should succeed (unique constraint on user)
	succeeded := 0
	for _, r := range results {
		if r {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Errorf("Expected exactly 1 successful registration for same user, got %d", succeeded)
	}
}

func TestRegisterWithInvite_EmptyPassword(t *testing.T) {
	store := newTestStore(t)

	code := "reg-code-nopass"
	err := store.CreateInvite(code, time.Now().Add(1*time.Hour), "admin")
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.RegisterWithInvite(code, "nopassuser", "")
	if err != nil {
		t.Fatalf("RegisterWithInvite with empty password failed: %v", err)
	}
	if !created {
		t.Error("Expected user to be created with empty password")
	}
}
