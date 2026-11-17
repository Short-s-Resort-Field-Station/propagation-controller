package api

import (
	"net/http"
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
