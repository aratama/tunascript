//go:build cgo
// +build cgo

package runtime

import "testing"

func TestResolveRoutePrefersExact(t *testing.T) {
	h1 := &Value{Kind: KindI64, I64: 1}
	h2 := &Value{Kind: KindI64, I64: 2}
	routes := map[string]*Value{
		"/run": h1,
		"/:id": h2,
	}

	handler, params, ok := resolveRoute("/run", routes)
	if !ok {
		t.Fatal("expected exact route to resolve")
	}
	if handler != h1 {
		t.Fatalf("expected exact route handler h1")
	}
	if len(params) != 0 {
		t.Fatalf("expected no params for exact route, got %+v", params)
	}
}

func TestResolveRouteMatchesPathParams(t *testing.T) {
	h := &Value{Kind: KindI64, I64: 10}
	routes := map[string]*Value{
		"/:id": h,
	}

	handler, params, ok := resolveRoute("/abc123", routes)
	if !ok {
		t.Fatal("expected param route to resolve")
	}
	if handler != h {
		t.Fatalf("expected handler h")
	}
	if got := params["id"]; got != "abc123" {
		t.Fatalf("expected id=abc123, got %q", got)
	}
}

func TestResolveRoutePrefersMoreSpecificPattern(t *testing.T) {
	h1 := &Value{Kind: KindI64, I64: 1}
	h2 := &Value{Kind: KindI64, I64: 2}
	routes := map[string]*Value{
		"/:id":     h1,
		"/run/:id": h2,
	}

	handler, params, ok := resolveRoute("/run/xyz", routes)
	if !ok {
		t.Fatal("expected route to resolve")
	}
	if handler != h2 {
		t.Fatalf("expected /run/:id handler h2")
	}
	if got := params["id"]; got != "xyz" {
		t.Fatalf("expected id=xyz, got %q", got)
	}
}

func TestResolveRouteReturnsNotFound(t *testing.T) {
	routes := map[string]*Value{
		"/run/:id": {Kind: KindI64, I64: 2},
	}

	_, _, ok := resolveRoute("/unknown", routes)
	if ok {
		t.Fatal("expected not found")
	}
}

func TestResolveRouteByMethodPrefersMethodSpecific(t *testing.T) {
	h1 := &Value{Kind: KindI64, I64: 1}
	h2 := &Value{Kind: KindI64, I64: 2}
	routes := map[string]map[string]*Value{
		routeMethodAny: {
			"/items/:id": h1,
		},
		"GET": {
			"/items/:id": h2,
		},
	}

	handler, params, ok := resolveRouteByMethod("/items/42", "GET", routes)
	if !ok {
		t.Fatal("expected route to resolve")
	}
	if handler != h2 {
		t.Fatalf("expected method-specific handler h2")
	}
	if got := params["id"]; got != "42" {
		t.Fatalf("expected id=42, got %q", got)
	}
}

func TestResolveRouteByMethodFallsBackToWildcard(t *testing.T) {
	h := &Value{Kind: KindI64, I64: 7}
	routes := map[string]map[string]*Value{
		routeMethodAny: {
			"/items/:id": h,
		},
	}

	handler, params, ok := resolveRouteByMethod("/items/abc", "POST", routes)
	if !ok {
		t.Fatal("expected wildcard route to resolve")
	}
	if handler != h {
		t.Fatalf("expected wildcard handler h")
	}
	if got := params["id"]; got != "abc" {
		t.Fatalf("expected id=abc, got %q", got)
	}
}
