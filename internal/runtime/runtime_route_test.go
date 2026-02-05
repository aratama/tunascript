//go:build cgo
// +build cgo

package runtime

import "testing"

func TestResolveRoutePrefersExact(t *testing.T) {
	routes := map[string]int32{
		"/run": 1,
		"/:id": 2,
	}

	handler, params, ok := resolveRoute("/run", routes)
	if !ok {
		t.Fatal("expected exact route to resolve")
	}
	if handler != 1 {
		t.Fatalf("expected exact route handler=1, got %d", handler)
	}
	if len(params) != 0 {
		t.Fatalf("expected no params for exact route, got %+v", params)
	}
}

func TestResolveRouteMatchesPathParams(t *testing.T) {
	routes := map[string]int32{
		"/:id": 10,
	}

	handler, params, ok := resolveRoute("/abc123", routes)
	if !ok {
		t.Fatal("expected param route to resolve")
	}
	if handler != 10 {
		t.Fatalf("expected handler=10, got %d", handler)
	}
	if got := params["id"]; got != "abc123" {
		t.Fatalf("expected id=abc123, got %q", got)
	}
}

func TestResolveRoutePrefersMoreSpecificPattern(t *testing.T) {
	routes := map[string]int32{
		"/:id":     1,
		"/run/:id": 2,
	}

	handler, params, ok := resolveRoute("/run/xyz", routes)
	if !ok {
		t.Fatal("expected route to resolve")
	}
	if handler != 2 {
		t.Fatalf("expected /run/:id handler=2, got %d", handler)
	}
	if got := params["id"]; got != "xyz" {
		t.Fatalf("expected id=xyz, got %q", got)
	}
}

func TestResolveRouteReturnsNotFound(t *testing.T) {
	routes := map[string]int32{
		"/run/:id": 2,
	}

	_, _, ok := resolveRoute("/unknown", routes)
	if ok {
		t.Fatal("expected not found")
	}
}

func TestResolveRouteByMethodPrefersMethodSpecific(t *testing.T) {
	routes := map[string]map[string]int32{
		routeMethodAny: {
			"/items/:id": 1,
		},
		"GET": {
			"/items/:id": 2,
		},
	}

	handler, params, ok := resolveRouteByMethod("/items/42", "GET", routes)
	if !ok {
		t.Fatal("expected route to resolve")
	}
	if handler != 2 {
		t.Fatalf("expected method-specific handler=2, got %d", handler)
	}
	if got := params["id"]; got != "42" {
		t.Fatalf("expected id=42, got %q", got)
	}
}

func TestResolveRouteByMethodFallsBackToWildcard(t *testing.T) {
	routes := map[string]map[string]int32{
		routeMethodAny: {
			"/items/:id": 7,
		},
	}

	handler, params, ok := resolveRouteByMethod("/items/abc", "POST", routes)
	if !ok {
		t.Fatal("expected wildcard route to resolve")
	}
	if handler != 7 {
		t.Fatalf("expected wildcard handler=7, got %d", handler)
	}
	if got := params["id"]; got != "abc" {
		t.Fatalf("expected id=abc, got %q", got)
	}
}
