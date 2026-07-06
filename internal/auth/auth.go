// Package auth implements local authentication (spec §20): bcrypt-hashed
// passwords, opaque session tokens (SHA-256 hashes at rest), and the three
// roles. The public internet sees the read-only view; control requires a
// signed-in operator; configuration requires an admin.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"propagation-controller/internal/logging"
	"propagation-controller/internal/storage"
)

const SessionTTL = 30 * 24 * time.Hour

// Role levels: higher may do everything lower may.
const (
	RoleViewer   = "viewer"
	RoleOperator = "operator"
	RoleAdmin    = "admin"
)

func RoleLevel(role string) int {
	switch role {
	case RoleAdmin:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

func ValidRole(role string) bool { return RoleLevel(role) > 0 }

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9._ \-]{2,40}$`)

type Service struct {
	users    *storage.UserRepo
	sessions *storage.SessionRepo
	events   *logging.EventLogger
}

func NewService(users *storage.UserRepo, sessions *storage.SessionRepo, events *logging.EventLogger) *Service {
	return &Service{users: users, sessions: sessions, events: events}
}

// SetupRequired reports whether no accounts exist yet (first boot).
func (s *Service) SetupRequired() bool {
	n, err := s.users.Count()
	return err == nil && n == 0
}

// Setup creates the first admin account. It only works while no users exist.
func (s *Service) Setup(username, password string) error {
	if !s.SetupRequired() {
		return fmt.Errorf("setup already completed — sign in as an admin to add operators")
	}
	if err := s.CreateUser(username, password, RoleAdmin); err != nil {
		return err
	}
	s.events.Info("auth_setup_complete", "first admin account created: "+username, "system", nil)
	return nil
}

func validateCredentials(username, password string) error {
	if !usernameRe.MatchString(strings.TrimSpace(username)) {
		return fmt.Errorf("username must be 2–40 characters (letters, numbers, . _ - and spaces)")
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	return nil
}

func (s *Service) CreateUser(username, password, role string) error {
	username = strings.TrimSpace(username)
	if err := validateCredentials(username, password); err != nil {
		return err
	}
	if !ValidRole(role) {
		return fmt.Errorf("role must be admin, operator, or viewer")
	}
	if existing, err := s.users.GetByUsername(username); err != nil {
		return err
	} else if existing != nil {
		return fmt.Errorf("username %q is taken", username)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.users.Insert(username, string(hash), role); err != nil {
		return err
	}
	s.events.Info("user_created", "operator account created: "+username+" ("+role+")", "system", nil)
	return nil
}

// Login verifies credentials and mints a session token (returned raw; only
// its hash is stored).
func (s *Service) Login(username, password string) (*storage.User, string, error) {
	user, err := s.users.GetByUsername(strings.TrimSpace(username))
	if err != nil {
		return nil, "", err
	}
	// Constant-ish behavior for unknown users: still run a bcrypt compare.
	hash := "$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5B0S6XN9K1PZaZqZQbW3Jm0eXGxKe"
	if user != nil {
		hash = user.PasswordHash
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil || user == nil || !user.Enabled {
		return nil, "", fmt.Errorf("invalid username or password")
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", err
	}
	token := hex.EncodeToString(raw)
	if err := s.sessions.Insert(user.ID, hashToken(token), time.Now().Add(SessionTTL)); err != nil {
		return nil, "", err
	}
	s.sessions.DeleteExpired()
	s.events.Info("user_signed_in", username+" signed in", username, nil)
	return user, token, nil
}

func (s *Service) Logout(token string) {
	s.sessions.Delete(hashToken(token))
}

// Resolve returns the user for a session token, or nil.
func (s *Service) Resolve(token string) *storage.User {
	if token == "" {
		return nil
	}
	user, err := s.sessions.ResolveUser(hashToken(token))
	if err != nil {
		return nil
	}
	return user
}

func (s *Service) ListUsers() ([]storage.User, error) { return s.users.List() }

func (s *Service) UpdateUser(username string, role *string, enabled *bool, newPassword *string, actor string) error {
	user, err := s.users.GetByUsername(username)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("unknown user %q", username)
	}
	if role != nil {
		if !ValidRole(*role) {
			return fmt.Errorf("role must be admin, operator, or viewer")
		}
		if user.Role == RoleAdmin && *role != RoleAdmin {
			if err := s.guardLastAdmin(username); err != nil {
				return err
			}
		}
		if err := s.users.SetRole(username, *role); err != nil {
			return err
		}
	}
	if enabled != nil {
		if !*enabled {
			if user.Role == RoleAdmin {
				if err := s.guardLastAdmin(username); err != nil {
					return err
				}
			}
			s.sessions.DeleteForUser(user.ID)
		}
		if err := s.users.SetEnabled(username, *enabled); err != nil {
			return err
		}
	}
	if newPassword != nil {
		if len(*newPassword) < 8 {
			return fmt.Errorf("password must be at least 8 characters")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*newPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err := s.users.SetPassword(username, string(hash)); err != nil {
			return err
		}
		s.sessions.DeleteForUser(user.ID)
	}
	s.events.Info("user_updated", "operator account updated: "+username, actor, nil)
	return nil
}

func (s *Service) DeleteUser(username, actor string) error {
	user, err := s.users.GetByUsername(username)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("unknown user %q", username)
	}
	if user.Role == RoleAdmin {
		if err := s.guardLastAdmin(username); err != nil {
			return err
		}
	}
	s.sessions.DeleteForUser(user.ID)
	if err := s.users.Delete(username); err != nil {
		return err
	}
	s.events.Warn("user_deleted", "operator account deleted: "+username, actor, nil)
	return nil
}

// guardLastAdmin refuses to demote/disable/delete the only enabled admin —
// the unit must never become unadministrable.
func (s *Service) guardLastAdmin(username string) error {
	users, err := s.users.List()
	if err != nil {
		return err
	}
	admins := 0
	for _, u := range users {
		if u.Role == RoleAdmin && u.Enabled && u.Username != username {
			admins++
		}
	}
	if admins == 0 {
		return fmt.Errorf("cannot remove the last admin account")
	}
	return nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
