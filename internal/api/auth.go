package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"propagation-controller/internal/auth"
	"propagation-controller/internal/storage"
)

const sessionCookie = "propagation_session"

type ctxKey int

const userKey ctxKey = 0

type requestUser struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	Implicit bool   `json:"implicit,omitempty"` // rack console (loopback), not a session
}

// resolveUser identifies the caller: a session cookie, or the rack console.
//
// The kiosk browser runs on the unit itself and talks straight to
// localhost, so loopback requests WITHOUT proxy forwarding headers are
// trusted as the rack console (admin — physical access to the rack is
// physical access to everything). Any reverse proxy (cloudflared, Caddy)
// adds X-Forwarded-For, so internet traffic arriving via localhost proxies
// is never mistaken for the rack.
func (s *Server) resolveUser(r *http.Request) *requestUser {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if u := s.auth.Resolve(c.Value); u != nil {
			return &requestUser{Username: u.Username, Role: u.Role}
		}
	}
	if r.Header.Get("X-Forwarded-For") == "" && r.Header.Get("Cf-Connecting-Ip") == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
				return &requestUser{Username: "Rack Console", Role: auth.RoleAdmin, Implicit: true}
			}
		}
	}
	return nil
}

// mutationRole maps non-GET API calls to the minimum role that may perform
// them (spec §20.2). Unlisted mutations default to admin.
func mutationRole(method, path string) string {
	p := strings.TrimPrefix(path, "/api/v1")
	switch {
	case strings.HasPrefix(p, "/auth/"):
		return "" // login/logout/setup manage themselves
	case strings.HasPrefix(p, "/relays/"),
		strings.HasPrefix(p, "/lighting/on"),
		strings.HasPrefix(p, "/lighting/off"),
		strings.HasPrefix(p, "/lighting/override"),
		strings.HasPrefix(p, "/fans/on"),
		strings.HasPrefix(p, "/fans/off"),
		strings.HasPrefix(p, "/fans/override"),
		strings.HasPrefix(p, "/diagnostics/"),
		strings.HasSuffix(p, "/poll"),
		strings.HasSuffix(p, "/run"),
		strings.HasSuffix(p, "/drain"),
		strings.HasSuffix(p, "/stop"),
		p == "/safety/emergency-stop":
		return auth.RoleOperator
	default:
		return auth.RoleAdmin
	}
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := s.resolveUser(r)
		r = r.WithContext(context.WithValue(r.Context(), userKey, user))

		// Reads are public: curious followers see everything, touch nothing.
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		required := mutationRole(r.Method, r.URL.Path)
		if required == "" {
			next.ServeHTTP(w, r)
			return
		}
		if user == nil {
			writeError(w, http.StatusUnauthorized, "sign in required to control the unit")
			return
		}
		if auth.RoleLevel(user.Role) < auth.RoleLevel(required) {
			writeError(w, http.StatusForbidden, "your role ("+user.Role+") cannot perform this action — "+required+" required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func currentUser(r *http.Request) *requestUser {
	u, _ := r.Context().Value(userKey).(*requestUser)
	return u
}

// --- handlers ---

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		MaxAge:   maxAge,
	})
}

func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"user":           currentUser(r),
		"setup_required": s.auth.SetupRequired(),
	})
}

// setupAllowedFrom: first-boot setup may only be claimed from the unit
// itself (the kiosk, or SSH + curl) — never through a tunnel/proxy (which
// always adds forwarding headers) and not from the LAN either: the unit sits
// on an IoT subnet, and any device there could otherwise claim an unset-up
// unit as its own (audit 2026-11-10, 1.2). The "no users yet" rule still
// applies on top of this, in auth.Setup.
func setupAllowedFrom(r *http.Request) bool {
	if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("Cf-Connecting-Ip") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) handleAuthSetup(w http.ResponseWriter, r *http.Request) {
	if !setupAllowedFrom(r) {
		writeError(w, http.StatusForbidden,
			"first-time setup must be run on the unit itself (rack console or localhost)")
		return
	}
	var req struct{ Username, Password string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.auth.Setup(req.Username, req.Password); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	// Sign the new admin straight in.
	_, token, err := s.auth.Login(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.setSessionCookie(w, r, token, int(auth.SessionTTL.Seconds()))
	writeJSON(w, http.StatusOK, map[string]any{
		"user": requestUser{Username: req.Username, Role: auth.RoleAdmin},
	})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var req struct{ Username, Password string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user, token, err := s.auth.Login(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	s.setSessionCookie(w, r, token, int(auth.SessionTTL.Seconds()))
	writeJSON(w, http.StatusOK, map[string]any{
		"user": requestUser{Username: user.Username, Role: user.Role},
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.auth.Logout(c.Value)
	}
	s.setSessionCookie(w, r, "", -1)
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed_out"})
}

type userView struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	Enabled  bool   `json:"enabled"`
}

func toUserViews(users []storage.User) []userView {
	out := make([]userView, 0, len(users))
	for _, u := range users {
		out = append(out, userView{Username: u.Username, Role: u.Role, Enabled: u.Enabled})
	}
	return out
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) *requestUser {
	u := currentUser(r)
	if u == nil || auth.RoleLevel(u.Role) < auth.RoleLevel(auth.RoleAdmin) {
		writeError(w, http.StatusForbidden, "admin required")
		return nil
	}
	return u
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	users, err := s.auth.ListUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": toUserViews(users)})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	actor := s.requireAdmin(w, r)
	if actor == nil {
		return
	}
	var req struct{ Username, Password, Role string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := s.auth.CreateUser(req.Username, req.Password, req.Role); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "created"})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	actor := s.requireAdmin(w, r)
	if actor == nil {
		return
	}
	var req struct {
		Role     *string `json:"role"`
		Enabled  *bool   `json:"enabled"`
		Password *string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	username := chi.URLParam(r, "username")
	if err := s.auth.UpdateUser(username, req.Role, req.Enabled, req.Password, actor.Username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	actor := s.requireAdmin(w, r)
	if actor == nil {
		return
	}
	if err := s.auth.DeleteUser(chi.URLParam(r, "username"), actor.Username); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
