package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/sessions"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"

	"github.com/clawbake/clawbake/internal/database"
)

type contextKey string

const (
	sessionName       = "clawbake-session"
	sessionKeyUserID  = "user_id"
	sessionKeyState   = "oauth_state"
	UserContextKey    = contextKey("user")
)

type OIDCAuth struct {
	provider     *oidc.Provider
	oauth2Config oauth2.Config
	verifier     *oidc.IDTokenVerifier
	store        sessions.Store
	db           *database.Queries
}

func NewOIDCAuth(ctx context.Context, issuer, clientID, clientSecret, redirectURL, sessionSecret, baseURL string, db *database.Queries) (*OIDCAuth, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}

	oauth2Config := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: clientID})
	store := sessions.NewCookieStore([]byte(sessionSecret))
	store.Options.SameSite = http.SameSiteLaxMode
	store.Options.Secure = strings.HasPrefix(baseURL, "https://")

	return &OIDCAuth{
		provider:     provider,
		oauth2Config: oauth2Config,
		verifier:     verifier,
		store:        store,
		db:           db,
	}, nil
}

func (a *OIDCAuth) LoginHandler(c echo.Context) error {
	state, err := randomState()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate state")
	}

	session, _ := a.store.Get(c.Request(), sessionName)
	session.Values[sessionKeyState] = state
	if err := session.Save(c.Request(), c.Response()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save session")
	}

	return c.Redirect(http.StatusFound, a.oauth2Config.AuthCodeURL(state))
}

type oidcClaims struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
	Sub     string `json:"sub"`
}

func (a *OIDCAuth) CallbackHandler(c echo.Context) error {
	session, _ := a.store.Get(c.Request(), sessionName)

	savedState, ok := session.Values[sessionKeyState].(string)
	if !ok || savedState == "" || savedState != c.QueryParam("state") {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid oauth state")
	}
	delete(session.Values, sessionKeyState)

	token, err := a.oauth2Config.Exchange(c.Request().Context(), c.QueryParam("code"))
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "failed to exchange code")
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "missing id_token")
	}

	idToken, err := a.verifier.Verify(c.Request().Context(), rawIDToken)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid id_token")
	}

	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to parse claims")
	}

	user, err := a.db.GetUserByOIDCSubject(c.Request().Context(), claims.Sub)
	if err != nil {
		// OIDC subject not found — try matching by email (e.g. Slack bot created user first)
		user, err = a.db.GetUserByEmail(c.Request().Context(), claims.Email)
		if err == nil {
			// Link the OIDC subject to the existing user
			user, err = a.db.UpdateUser(c.Request().Context(), database.UpdateUserParams{
				ID:          user.ID,
				OidcSubject: pgtype.Text{String: claims.Sub, Valid: true},
			})
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to link user")
			}
		} else {
			user, err = a.db.CreateUser(c.Request().Context(), database.CreateUserParams{
				Email:       claims.Email,
				Name:        claims.Name,
				Picture:     pgtype.Text{String: claims.Picture, Valid: claims.Picture != ""},
				Role:        "user",
				OidcSubject: claims.Sub,
			})
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to create user")
			}
		}
	}

	userID, _ := user.ID.Value()
	session.Values[sessionKeyUserID] = userID
	if err := session.Save(c.Request(), c.Response()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save session")
	}

	return c.Redirect(http.StatusFound, "/")
}

func (a *OIDCAuth) LogoutHandler(c echo.Context) error {
	session, _ := a.store.Get(c.Request(), sessionName)
	session.Options.MaxAge = -1
	_ = session.Save(c.Request(), c.Response())
	return c.Redirect(http.StatusFound, "/")
}

func (a *OIDCAuth) RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		session, _ := a.store.Get(c.Request(), sessionName)

		userIDVal, ok := session.Values[sessionKeyUserID]
		if !ok {
			return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
		}

		userIDStr, ok := userIDVal.(string)
		if !ok {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
		}

		var uid pgtype.UUID
		if err := uid.Scan(userIDStr); err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
		}

		user, err := a.db.GetUserByID(c.Request().Context(), uid)
		if err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
		}

		ctx := context.WithValue(c.Request().Context(), UserContextKey, &user)
		c.SetRequest(c.Request().WithContext(ctx))
		return next(c)
	}
}

// OptionalAuth sets the user in context if a valid session exists but does
// not block the request when no session is present.
func (a *OIDCAuth) OptionalAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		session, _ := a.store.Get(c.Request(), sessionName)
		userIDVal, ok := session.Values[sessionKeyUserID]
		if ok {
			userIDStr, ok := userIDVal.(string)
			if ok {
				var uid pgtype.UUID
				if err := uid.Scan(userIDStr); err == nil {
					user, err := a.db.GetUserByID(c.Request().Context(), uid)
					if err == nil {
						ctx := context.WithValue(c.Request().Context(), UserContextKey, &user)
						c.SetRequest(c.Request().WithContext(ctx))
					}
				}
			}
		}
		return next(c)
	}
}

func (a *OIDCAuth) RequireAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user := UserFromContext(c.Request().Context())
		if user == nil || user.Role != "admin" {
			return echo.NewHTTPError(http.StatusForbidden, "admin access required")
		}
		return next(c)
	}
}

func UserFromContext(ctx context.Context) *database.User {
	user, _ := ctx.Value(UserContextKey).(*database.User)
	return user
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ErrNoSession is returned when no valid session exists.
var ErrNoSession = errors.New("no valid session")

// WithDevUser injects a synthetic admin user into the context for local
// development when OIDC is not configured.
func WithDevUser(ctx context.Context) context.Context {
	devUser := &database.User{
		ID: pgtype.UUID{
			Bytes: [16]byte{0xde, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			Valid: true,
		},
		Email: "dev@localhost",
		Name:  "Dev User",
		Role:  "admin",
	}
	return context.WithValue(ctx, UserContextKey, devUser)
}
