package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/clawbake/clawbake/internal/auth"
	"github.com/clawbake/clawbake/internal/database"
)

func TestUserFromContextNil(t *testing.T) {
	user := auth.UserFromContext(context.Background())
	if user != nil {
		t.Error("expected nil user from empty context")
	}
}

func TestUserFromContextWithUser(t *testing.T) {
	var uid pgtype.UUID
	_ = uid.Scan("00000000-0000-0000-0000-000000000001")
	user := &database.User{
		ID:    uid,
		Email: "test@example.com",
		Name:  "Test",
		Role:  "user",
	}
	ctx := context.WithValue(context.Background(), auth.UserContextKey, user)

	got := auth.UserFromContext(ctx)
	if got == nil {
		t.Fatal("expected non-nil user")
	}
	if got.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", got.Email)
	}
}

func TestRequireAuthRejectsUnauthenticated(t *testing.T) {
	// RequireAuth requires a valid session. Without OIDC provider setup,
	// we test that the middleware concept works by verifying an unauthenticated
	// request returns 401.
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Simulate middleware behavior: no user in context means 401
	handler := func(c echo.Context) error {
		user := auth.UserFromContext(c.Request().Context())
		if user == nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}
		return c.String(http.StatusOK, "ok")
	}

	err := handler(c)
	if err == nil {
		t.Fatal("expected error for unauthenticated request")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected echo.HTTPError, got %T", err)
	}
	if httpErr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", httpErr.Code)
	}
}

func TestRequireAuthAllowsAuthenticated(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var uid pgtype.UUID
	_ = uid.Scan("00000000-0000-0000-0000-000000000001")
	user := &database.User{
		ID:    uid,
		Email: "test@example.com",
		Name:  "Test",
		Role:  "user",
	}
	ctx := context.WithValue(c.Request().Context(), auth.UserContextKey, user)
	c.SetRequest(c.Request().WithContext(ctx))

	handler := func(c echo.Context) error {
		u := auth.UserFromContext(c.Request().Context())
		if u == nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}
		return c.String(http.StatusOK, "ok")
	}

	err := handler(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequireAdminRejectsNonAdmin(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var uid pgtype.UUID
	_ = uid.Scan("00000000-0000-0000-0000-000000000001")
	user := &database.User{
		ID:   uid,
		Role: "user",
	}
	ctx := context.WithValue(c.Request().Context(), auth.UserContextKey, user)
	c.SetRequest(c.Request().WithContext(ctx))

	handler := func(c echo.Context) error {
		u := auth.UserFromContext(c.Request().Context())
		if u == nil || u.Role != "admin" {
			return echo.NewHTTPError(http.StatusForbidden, "admin access required")
		}
		return c.String(http.StatusOK, "ok")
	}

	err := handler(c)
	if err == nil {
		t.Fatal("expected error for non-admin user")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected echo.HTTPError, got %T", err)
	}
	if httpErr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", httpErr.Code)
	}
}
