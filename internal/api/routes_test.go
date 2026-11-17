package api

import (
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestScheduledModesHaveRoutes: the dashboard and the rack panel post
// /lighting/{on,off,auto} and /fans/{on,off,auto}. Until 2026-11-17 the two
// "auto" routes were missing, so Auto answered 404 and an override could not
// be cleared from either screen.
func TestScheduledModesHaveRoutes(t *testing.T) {
	routes := newRouter(Deps{})
	have := map[string]bool{}
	_ = chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodPost {
			have[route] = true
		}
		return nil
	})
	for _, svc := range []string{"lighting", "fans"} {
		for _, mode := range []string{"on", "off", "auto"} {
			found := false
			for route := range have {
				if len(route) >= len("/"+svc+"/"+mode) && route[len(route)-len("/"+svc+"/"+mode):] == "/"+svc+"/"+mode {
					found = true
				}
			}
			if !found {
				t.Errorf("no POST route for /%s/%s", svc, mode)
			}
		}
	}
}

// TestRouteTableGolden pins the API route table to testdata/routes.txt. The
// frontend's contract test (web/src/live/api.contract.test.js) reads the same
// file and fails if any call in api.js has no route, so a button can no longer
// post to a path the daemon does not serve. After adding or removing a route:
//
//	UPDATE_GOLDEN=1 go test ./internal/api/ -run TestRouteTableGolden
func TestRouteTableGolden(t *testing.T) {
	var got []string
	_ = chi.Walk(newRouter(Deps{}), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/api/v1/") {
			got = append(got, method+" "+route)
		}
		return nil
	})
	sort.Strings(got)
	text := strings.Join(got, "\n") + "\n"
	const path = "testdata/routes.txt"
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run with UPDATE_GOLDEN=1)", err)
	}
	if string(want) != text {
		t.Errorf("route table changed; review the diff, then regenerate %s with UPDATE_GOLDEN=1\n--- got ---\n%s", path, text)
	}
}

// TestScheduledModesShareARole: whoever may force lights or fans on or off
// may also hand them back to the schedule. /lighting/auto and /fans/auto were
// added on 2026-11-17 without this, which left Auto admin-only: a Grower
// could force the lights and then not undo it.
func TestScheduledModesShareARole(t *testing.T) {
	for _, svc := range []string{"lighting", "fans"} {
		want := mutationRole(http.MethodPost, "/api/v1/"+svc+"/on")
		for _, mode := range []string{"off", "auto", "override"} {
			if got := mutationRole(http.MethodPost, "/api/v1/"+svc+"/"+mode); got != want {
				t.Errorf("/%s/%s needs role %q, but /%s/on needs %q", svc, mode, got, svc, want)
			}
		}
	}
}

// TestResetSharesTheEstopRole: whoever may hit E-STOP may reset it.
func TestResetSharesTheEstopRole(t *testing.T) {
	stop := mutationRole(http.MethodPost, "/api/v1/safety/emergency-stop")
	if got := mutationRole(http.MethodPost, "/api/v1/safety/reset"); got != stop {
		t.Errorf("/safety/reset needs %q, /safety/emergency-stop needs %q", got, stop)
	}
}
