package storage

import (
	"testing"
	"time"
)

func TestCreateInvite_Valid(t *testing.T) {
	store := newTestStore(t)

	code := "testcode123"
	expiresAt := time.Now().Add(1 * time.Hour)
	err := store.CreateInvite(code, expiresAt, "admin")
	if err != nil {
		t.Fatalf("Failed to create invite: %v", err)
	}

	valid, err := store.ValidateInvite(code)
	if err != nil {
		t.Fatalf("Failed to validate invite: %v", err)
	}
	if !valid {
		t.Error("Expected valid invite to be valid")
	}
}

func TestCreateInvite_Nonexistent(t *testing.T) {
	store := newTestStore(t)

	valid, err := store.ValidateInvite("nonexistent")
	if err != nil {
		t.Fatalf("Failed to validate invite: %v", err)
	}
	if valid {
		t.Error("Expected nonexistent invite to be invalid")
	}
}

func TestCreateInvite_Expired(t *testing.T) {
	store := newTestStore(t)

	code := "expired-code"
	expiresAt := time.Now().Add(-1 * time.Hour) // 1 hour ago
	err := store.CreateInvite(code, expiresAt, "admin")
	if err != nil {
		t.Fatalf("Failed to create invite: %v", err)
	}

	valid, err := store.ValidateInvite(code)
	if err != nil {
		t.Fatalf("Failed to validate invite: %v", err)
	}
	if valid {
		t.Error("Expected expired invite to be invalid")
	}
}

func TestMarkInviteUsed_SingleUse(t *testing.T) {
	store := newTestStore(t)

	code := "single-use-code"
	expiresAt := time.Now().Add(1 * time.Hour)
	err := store.CreateInvite(code, expiresAt, "admin")
	if err != nil {
		t.Fatalf("Failed to create invite: %v", err)
	}

	// First use should succeed
	err = store.MarkInviteUsed(code, "alice")
	if err != nil {
		t.Fatalf("Failed to mark invite used: %v", err)
	}

	// Invite should now be invalid (used_at IS NULL check)
	valid, err := store.ValidateInvite(code)
	if err != nil {
		t.Fatalf("Failed to validate used invite: %v", err)
	}
	if valid {
		t.Error("Expected used invite to be invalid")
	}
}

func TestMarkInviteUsed_DoubleUse(t *testing.T) {
	store := newTestStore(t)

	code := "double-use-code"
	expiresAt := time.Now().Add(1 * time.Hour)
	err := store.CreateInvite(code, expiresAt, "admin")
	if err != nil {
		t.Fatalf("Failed to create invite: %v", err)
	}

	// First use
	err = store.MarkInviteUsed(code, "alice")
	if err != nil {
		t.Fatalf("Failed to mark invite used: %v", err)
	}

	// Second use — should error (no rows match the WHERE clause)
	err = store.MarkInviteUsed(code, "bob")
	if err == nil {
		t.Error("Expected error when marking already-used invite, got nil")
	}
}

func TestMarkInviteUsed_AuditTrail(t *testing.T) {
	store := newTestStore(t)

	code := "audit-code"
	expiresAt := time.Now().Add(1 * time.Hour)
	err := store.CreateInvite(code, expiresAt, "admin_user")
	if err != nil {
		t.Fatalf("Failed to create invite: %v", err)
	}

	// Mark used by a different user
	err = store.MarkInviteUsed(code, "new_user")
	if err != nil {
		t.Fatalf("Failed to mark invite used: %v", err)
	}

	// Query the invite row directly to verify both columns
	var createdBy, usedBy string
	err = store.db.QueryRow(
		"SELECT created_by_user, used_by_user FROM invites WHERE code = ?", code,
	).Scan(&createdBy, &usedBy)
	if err != nil {
		t.Fatalf("Failed to query invite: %v", err)
	}

	if createdBy != "admin_user" {
		t.Errorf("Expected created_by_user='admin_user', got %q", createdBy)
	}
	if usedBy != "new_user" {
		t.Errorf("Expected used_by_user='new_user', got %q", usedBy)
	}
}

func TestMultipleInvites_Independent(t *testing.T) {
	store := newTestStore(t)

	// Create two invites
	err := store.CreateInvite("code-a", time.Now().Add(1*time.Hour), "admin")
	if err != nil {
		t.Fatalf("Failed to create invite A: %v", err)
	}
	err = store.CreateInvite("code-b", time.Now().Add(1*time.Hour), "admin")
	if err != nil {
		t.Fatalf("Failed to create invite B: %v", err)
	}

	// Use code A
	err = store.MarkInviteUsed("code-a", "alice")
	if err != nil {
		t.Fatalf("Failed to mark invite A used: %v", err)
	}

	// Code A should be invalid, code B should still be valid
	validA, err := store.ValidateInvite("code-a")
	if err != nil {
		t.Fatalf("Failed to validate code A: %v", err)
	}
	if validA {
		t.Error("Expected used invite A to be invalid")
	}

	validB, err := store.ValidateInvite("code-b")
	if err != nil {
		t.Fatalf("Failed to validate code B: %v", err)
	}
	if !validB {
		t.Error("Expected unused invite B to be valid")
	}
}

func TestMarkInviteUsed_Nonexistent(t *testing.T) {
	store := newTestStore(t)

	// Mark a nonexistent invite as used — should error
	err := store.MarkInviteUsed("nonexistent", "alice")
	if err == nil {
		t.Error("Expected error when marking nonexistent invite, got nil")
	}
}
