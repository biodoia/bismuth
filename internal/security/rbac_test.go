package security

import (
	"context"
	"testing"
)

func TestUserFromHeaders(t *testing.T) {
	u := UserFromHeaders("sergio@example.com", "Sergio")
	if u == nil {
		t.Fatal("expected user")
	}
	if u.Email != "sergio@example.com" {
		t.Fatalf("expected email=sergio@example.com, got %s", u.Email)
	}
	if u.Name != "Sergio" {
		t.Fatalf("expected name=Sergio, got %s", u.Name)
	}
	if u.Role != RoleAdmin {
		t.Fatalf("expected role=admin, got %s", u.Role)
	}
	if !u.Tailscale {
		t.Fatal("expected tailscale=true")
	}
}

func TestUserFromHeadersEmpty(t *testing.T) {
	u := UserFromHeaders("", "")
	if u != nil {
		t.Fatal("expected nil for empty headers")
	}
}

func TestUserRBAC(t *testing.T) {
	admin := &User{Role: RoleAdmin, Tailscale: true}
	op := &User{Role: RoleOperator, Tailscale: true}
	viewer := &User{Role: RoleViewer, Tailscale: true}

	if !admin.CanSpawn() || !admin.CanKill() || !admin.CanRead() {
		t.Fatal("admin should be able to do everything")
	}
	if !op.CanSpawn() || !op.CanKill() || !op.CanRead() {
		t.Fatal("operator should be able to spawn/kill/read")
	}
	if viewer.CanSpawn() || viewer.CanKill() || !viewer.CanRead() {
		t.Fatal("viewer should only be able to read")
	}
}

func TestNilUserRBAC(t *testing.T) {
	var u *User
	if u.CanSpawn() || u.CanKill() || u.CanRead() {
		t.Fatal("nil user should not be able to do anything")
	}
}

func TestContextWithUser(t *testing.T) {
	u := &User{Email: "test@test.com", Role: RoleAdmin}
	ctx := ContextWithUser(context.Background(), u)
	got := UserFromContext(ctx)
	if got == nil || got.Email != "test@test.com" {
		t.Fatal("expected to get user back from context")
	}
}

func TestUserFromContextMissing(t *testing.T) {
	got := UserFromContext(context.Background())
	if got != nil {
		t.Fatal("expected nil from empty context")
	}
}
