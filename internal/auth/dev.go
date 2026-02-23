package auth

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/clawbake/clawbake/internal/database"
)

const devSessionKeyRole = "dev_role"

type DevAuth struct {
	store sessions.Store
	db    *database.Queries
}

func NewDevAuth(sessionSecret string, db *database.Queries) *DevAuth {
	if sessionSecret == "" {
		sessionSecret = "dev-secret-key-not-for-production"
	}
	store := sessions.NewCookieStore([]byte(sessionSecret))
	store.Options.Secure = false
	store.Options.SameSite = http.SameSiteLaxMode
	return &DevAuth{
		store: store,
		db:    db,
	}
}

func (d *DevAuth) LoginHandler(c echo.Context) error {
	role := c.QueryParam("role")
	if role != "admin" && role != "user" {
		role = "user"
	}

	if _, err := d.ensureDevUser(c.Request().Context(), role); err != nil {
		log.Printf("warning: failed to ensure dev user in database: %v", err)
	}

	session, _ := d.store.Get(c.Request(), sessionName)
	session.Values[devSessionKeyRole] = role
	if err := session.Save(c.Request(), c.Response()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save session")
	}

	return c.Redirect(http.StatusFound, "/")
}

func (d *DevAuth) LogoutHandler(c echo.Context) error {
	session, _ := d.store.Get(c.Request(), sessionName)
	session.Options.MaxAge = -1
	_ = session.Save(c.Request(), c.Response())
	return c.Redirect(http.StatusFound, "/")
}

type devUserSpec struct {
	Email       string
	Name        string
	Role        string
	OIDCSubject string
}

func devSpecForRole(role string) devUserSpec {
	if role == "admin" {
		return devUserSpec{
			Email:       "dev-admin@localhost",
			Name:        "Dev Admin",
			Role:        "admin",
			OIDCSubject: "dev:admin",
		}
	}
	return devUserSpec{
		Email:       "dev-user@localhost",
		Name:        "Dev User",
		Role:        "user",
		OIDCSubject: "dev:user",
	}
}

// ensureDevUser gets or creates the dev user in the database.
func (d *DevAuth) ensureDevUser(ctx context.Context, role string) (*database.User, error) {
	spec := devSpecForRole(role)

	user, err := d.db.GetUserByEmail(ctx, spec.Email)
	if err == nil {
		return &user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	user, err = d.db.CreateUser(ctx, database.CreateUserParams{
		Email:       spec.Email,
		Name:        spec.Name,
		Picture:     pgtype.Text{},
		Role:        spec.Role,
		OidcSubject: spec.OIDCSubject,
	})
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (d *DevAuth) getDevUser(ctx context.Context, role string) *database.User {
	spec := devSpecForRole(role)
	user, err := d.db.GetUserByEmail(ctx, spec.Email)
	if err != nil {
		return &database.User{
			ID: pgtype.UUID{
				Bytes: [16]byte{0xde, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
				Valid: true,
			},
			Email: spec.Email,
			Name:  spec.Name,
			Role:  spec.Role,
		}
	}
	return &user
}

func (d *DevAuth) RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		session, _ := d.store.Get(c.Request(), sessionName)
		role, ok := session.Values[devSessionKeyRole].(string)
		if !ok {
			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}

		user := d.getDevUser(c.Request().Context(), role)
		ctx := context.WithValue(c.Request().Context(), UserContextKey, user)
		c.SetRequest(c.Request().WithContext(ctx))
		return next(c)
	}
}

func (d *DevAuth) OptionalAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		session, _ := d.store.Get(c.Request(), sessionName)
		role, ok := session.Values[devSessionKeyRole].(string)
		if ok {
			user := d.getDevUser(c.Request().Context(), role)
			ctx := context.WithValue(c.Request().Context(), UserContextKey, user)
			c.SetRequest(c.Request().WithContext(ctx))
		}
		return next(c)
	}
}

func (d *DevAuth) RequireAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user := UserFromContext(c.Request().Context())
		if user == nil || user.Role != "admin" {
			return echo.NewHTTPError(http.StatusForbidden, "admin access required")
		}
		return next(c)
	}
}
