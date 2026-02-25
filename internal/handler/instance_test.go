package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
	"github.com/clawbake/clawbake/internal/auth"
	"github.com/clawbake/clawbake/internal/config"
	"github.com/clawbake/clawbake/internal/database"
	"github.com/clawbake/clawbake/internal/handler"
	"github.com/clawbake/clawbake/internal/k8s"

	"github.com/jackc/pgx/v5/pgtype"
)

func newTestScheme() *runtime.Scheme {
	return k8s.NewScheme()
}

func newUserContext(ctx context.Context, userID string, role string) context.Context {
	var uid pgtype.UUID
	_ = uid.Scan(userID)
	user := &database.User{
		ID:    uid,
		Email: "test@example.com",
		Name:  "Test User",
		Role:  role,
	}
	return context.WithValue(ctx, auth.UserContextKey, user)
}

func TestListInstances(t *testing.T) {
	scheme := newTestScheme()
	userID := "00000000-0000-0000-0000-000000000001"

	existing := &v1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userID,
			Namespace: "clawbake",
		},
		Spec: v1alpha1.ClawInstanceSpec{
			UserId: userID,
			Image:  "openclaw:latest",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	h := &handler.Handler{
		K8s:    fakeClient,
		Config: &config.Config{KubeNamespace: "clawbake"},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	ctx := newUserContext(c.Request().Context(), userID, "user")
	c.SetRequest(c.Request().WithContext(ctx))

	if err := h.ListInstances(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var instances []v1alpha1.ClawInstance
	if err := json.Unmarshal(rec.Body.Bytes(), &instances); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(instances) != 1 {
		t.Errorf("expected 1 instance, got %d", len(instances))
	}
}

func TestListInstancesAdminSeesAll(t *testing.T) {
	scheme := newTestScheme()

	instances := []v1alpha1.ClawInstance{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "00000000-0000-0000-0000-000000000001",
				Namespace: "clawbake",
			},
			Spec: v1alpha1.ClawInstanceSpec{
				UserId: "00000000-0000-0000-0000-000000000001",
				Image:  "openclaw:latest",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "00000000-0000-0000-0000-000000000002",
				Namespace: "clawbake",
			},
			Spec: v1alpha1.ClawInstanceSpec{
				UserId: "00000000-0000-0000-0000-000000000002",
				Image:  "openclaw:latest",
			},
		},
	}

	objs := make([]runtime.Object, len(instances))
	for i := range instances {
		objs[i] = &instances[i]
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()

	h := &handler.Handler{
		K8s:    fakeClient,
		Config: &config.Config{KubeNamespace: "clawbake"},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	adminID := "00000000-0000-0000-0000-000000000099"
	ctx := newUserContext(c.Request().Context(), adminID, "admin")
	c.SetRequest(c.Request().WithContext(ctx))

	if err := h.ListInstances(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []v1alpha1.ClawInstance
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected admin to see 2 instances, got %d", len(result))
	}
}

func TestGetInstance(t *testing.T) {
	scheme := newTestScheme()
	userID := "00000000-0000-0000-0000-000000000001"

	existing := &v1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userID,
			Namespace: "clawbake",
		},
		Spec: v1alpha1.ClawInstanceSpec{
			UserId: userID,
			Image:  "openclaw:latest",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	h := &handler.Handler{
		K8s:    fakeClient,
		Config: &config.Config{KubeNamespace: "clawbake"},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/instances/"+userID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(userID)

	ctx := newUserContext(c.Request().Context(), userID, "user")
	c.SetRequest(c.Request().WithContext(ctx))

	if err := h.GetInstance(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestGetInstanceNotOwned(t *testing.T) {
	scheme := newTestScheme()

	existing := &v1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "00000000-0000-0000-0000-000000000002",
			Namespace: "clawbake",
		},
		Spec: v1alpha1.ClawInstanceSpec{
			UserId: "00000000-0000-0000-0000-000000000002",
			Image:  "openclaw:latest",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	h := &handler.Handler{
		K8s:    fakeClient,
		Config: &config.Config{KubeNamespace: "clawbake"},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/instances/00000000-0000-0000-0000-000000000002", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("00000000-0000-0000-0000-000000000002")

	otherUser := "00000000-0000-0000-0000-000000000001"
	ctx := newUserContext(c.Request().Context(), otherUser, "user")
	c.SetRequest(c.Request().WithContext(ctx))

	err := h.GetInstance(c)
	if err == nil {
		t.Fatal("expected error for non-owned instance")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected echo.HTTPError, got %T", err)
	}
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", httpErr.Code)
	}
}

func TestDeleteInstance(t *testing.T) {
	scheme := newTestScheme()
	userID := "00000000-0000-0000-0000-000000000001"

	existing := &v1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userID,
			Namespace: "clawbake",
		},
		Spec: v1alpha1.ClawInstanceSpec{
			UserId: userID,
			Image:  "openclaw:latest",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	h := &handler.Handler{
		K8s:    fakeClient,
		Config: &config.Config{KubeNamespace: "clawbake"},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/instances/"+userID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(userID)

	ctx := newUserContext(c.Request().Context(), userID, "user")
	c.SetRequest(c.Request().WithContext(ctx))

	if err := h.DeleteInstance(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", rec.Code)
	}
}

func TestCreateInstanceAlreadyExists(t *testing.T) {
	scheme := newTestScheme()
	userID := "00000000-0000-0000-0000-000000000001"

	existing := &v1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userID,
			Namespace: "clawbake",
		},
		Spec: v1alpha1.ClawInstanceSpec{
			UserId: userID,
			Image:  "openclaw:latest",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	h := &handler.Handler{
		K8s:    fakeClient,
		Config: &config.Config{KubeNamespace: "clawbake"},
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/instances", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	ctx := newUserContext(c.Request().Context(), userID, "user")
	c.SetRequest(c.Request().WithContext(ctx))

	err := h.CreateInstance(c)
	if err == nil {
		t.Fatal("expected error for already existing instance")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected echo.HTTPError, got %T", err)
	}
	if httpErr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", httpErr.Code)
	}
}
