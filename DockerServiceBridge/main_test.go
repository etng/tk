package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildLinkFromContainerLabels(t *testing.T) {
	link, ok := buildLinkFromContainer(containerInfo{
		ID:    "abc123",
		Names: []string{"/demo"},
		Labels: map[string]string{
			"homepage.group":       "SelfHost",
			"homepage.name":        "Demo",
			"homepage.href":        "http://127.0.0.1:8080",
			"homepage.description": "Local service",
			"homepage.icon":        "server",
		},
	})
	if !ok {
		t.Fatal("expected homepage labels to produce a link")
	}
	if link.ID != "docker:abc123" ||
		link.Title != "Demo" ||
		link.URL != "http://127.0.0.1:8080" ||
		link.Group != "SelfHost" ||
		link.Description != "Local service" ||
		link.Icon != "server" {
		t.Fatalf("unexpected link: %#v", link)
	}
}

func TestBuildLinkFromContainerRejectsMissingURL(t *testing.T) {
	_, ok := buildLinkFromContainer(containerInfo{
		ID: "abc123",
		Labels: map[string]string{
			"homepage.name": "Demo",
		},
	})
	if ok {
		t.Fatal("expected container without homepage href to be ignored")
	}
}

func TestServiceHandlerAllowsConfiguredOrigin(t *testing.T) {
	handler := newServiceHandler(serviceConfig{
		AllowedOrigins: map[string]struct{}{"https://etng.github.io": {}},
		ListContainers: func() ([]containerInfo, error) {
			return []containerInfo{{
				ID: "abc123",
				Labels: map[string]string{
					"homepage.name": "Demo",
					"homepage.href": "http://127.0.0.1:8080",
				},
			}}, nil
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/services", nil)
	request.Header.Set("Origin", "https://etng.github.io")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://etng.github.io" {
		t.Fatalf("unexpected CORS origin %q", got)
	}
	var body serviceFeed
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Links) != 1 || body.Links[0].Title != "Demo" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestServiceHandlerRejectsUntrustedOrigin(t *testing.T) {
	handler := newServiceHandler(serviceConfig{
		AllowedOrigins: map[string]struct{}{"https://etng.github.io": {}},
		ListContainers: func() ([]containerInfo, error) {
			return nil, nil
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/services", nil)
	request.Header.Set("Origin", "https://evil.example")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}

func TestServiceHandlerAllowsPrivateNetworkPreflight(t *testing.T) {
	handler := newServiceHandler(serviceConfig{
		AllowedOrigins: map[string]struct{}{"https://etng.github.io": {}},
		ListContainers: func() ([]containerInfo, error) {
			return nil, nil
		},
	})
	request := httptest.NewRequest(http.MethodOptions, "/v1/services", nil)
	request.Header.Set("Origin", "https://etng.github.io")
	request.Header.Set("Access-Control-Request-Private-Network", "true")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Fatalf("expected private network access header, got %q", got)
	}
}

func TestServiceHandlerAcceptsTokenQuery(t *testing.T) {
	handler := newServiceHandler(serviceConfig{
		Token: "secret",
		ListContainers: func() ([]containerInfo, error) {
			return nil, nil
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/healthz?token=secret", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
}

func TestValidateListenAddressRejectsNonLoopbackByDefault(t *testing.T) {
	if err := validateListenAddress("0.0.0.0:17321", false); err == nil {
		t.Fatal("expected non-loopback bind to be rejected by default")
	}
}

func TestValidateListenAddressAllowsContainerBindWhenExplicit(t *testing.T) {
	if err := validateListenAddress("0.0.0.0:17321", true); err != nil {
		t.Fatalf("expected explicit container bind to be allowed, got %v", err)
	}
}
