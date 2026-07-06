package auth

import (
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"propagation-controller/internal/logging"
	"propagation-controller/internal/storage"
)

func newService(t *testing.T) *Service {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := storage.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	events := logging.NewEventLogger(zerolog.Nop(), storage.NewEventRepo(db))
	return NewService(storage.NewUserRepo(db), storage.NewSessionRepo(db), events)
}

func TestSetupOnceAndLogin(t *testing.T) {
	s := newService(t)
	if !s.SetupRequired() {
		t.Fatal("fresh install should require setup")
	}
	if err := s.Setup("Austin", "propagate123"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if s.SetupRequired() {
		t.Error("setup should be complete")
	}
	if err := s.Setup("Mallory", "hackhackhack"); err == nil {
		t.Error("second setup must be rejected")
	}

	user, token, err := s.Login("Austin", "propagate123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if user.Role != RoleAdmin || token == "" {
		t.Errorf("login result: role=%s token empty=%v", user.Role, token == "")
	}

	if resolved := s.Resolve(token); resolved == nil || resolved.Username != "Austin" {
		t.Errorf("resolve failed: %+v", resolved)
	}
	if s.Resolve("bogus-token") != nil {
		t.Error("bogus token resolved")
	}

	if _, _, err := s.Login("Austin", "wrong-password"); err == nil {
		t.Error("wrong password accepted")
	}
	if _, _, err := s.Login("Nobody", "propagate123"); err == nil {
		t.Error("unknown user accepted")
	}

	s.Logout(token)
	if s.Resolve(token) != nil {
		t.Error("token survives logout")
	}
}

func TestUserManagementAndLastAdminGuard(t *testing.T) {
	s := newService(t)
	s.Setup("Austin", "propagate123")

	if err := s.CreateUser("M. Reyes", "growgrowgrow", RoleOperator); err != nil {
		t.Fatalf("create operator: %v", err)
	}
	if err := s.CreateUser("M. Reyes", "duplicate123", RoleOperator); err == nil {
		t.Error("duplicate username accepted")
	}
	if err := s.CreateUser("x", "short", RoleOperator); err == nil {
		t.Error("weak credentials accepted")
	}

	// Operator can sign in and is capped at operator level.
	op, _, err := s.Login("M. Reyes", "growgrowgrow")
	if err != nil || RoleLevel(op.Role) != RoleLevel(RoleOperator) {
		t.Fatalf("operator login: %v", err)
	}

	// Last-admin guard: cannot demote, disable, or delete the only admin.
	demote := RoleViewer
	if err := s.UpdateUser("Austin", &demote, nil, nil, "test"); err == nil {
		t.Error("demoting the last admin was allowed")
	}
	off := false
	if err := s.UpdateUser("Austin", nil, &off, nil, "test"); err == nil {
		t.Error("disabling the last admin was allowed")
	}
	if err := s.DeleteUser("Austin", "test"); err == nil {
		t.Error("deleting the last admin was allowed")
	}

	// A second admin unlocks changes to the first.
	if err := s.CreateUser("Backup Admin", "backupbackup", RoleAdmin); err != nil {
		t.Fatalf("create second admin: %v", err)
	}
	if err := s.UpdateUser("Austin", &demote, nil, nil, "test"); err != nil {
		t.Errorf("demote with backup admin present: %v", err)
	}

	// Disabling a user kills their sessions.
	_, opToken, _ := s.Login("M. Reyes", "growgrowgrow")
	if err := s.UpdateUser("M. Reyes", nil, &off, nil, "test"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if s.Resolve(opToken) != nil {
		t.Error("disabled user's session still valid")
	}
	if _, _, err := s.Login("M. Reyes", "growgrowgrow"); err == nil {
		t.Error("disabled user can log in")
	}

	// Password change kills sessions and old password.
	newPw := "freshpassword"
	on := true
	s.UpdateUser("M. Reyes", nil, &on, nil, "test")
	if err := s.UpdateUser("M. Reyes", nil, nil, &newPw, "test"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	if _, _, err := s.Login("M. Reyes", "growgrowgrow"); err == nil {
		t.Error("old password still works")
	}
	if _, _, err := s.Login("M. Reyes", newPw); err != nil {
		t.Errorf("new password rejected: %v", err)
	}
}
