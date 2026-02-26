package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
	"github.com/clawbake/clawbake/internal/auth"
	"github.com/clawbake/clawbake/internal/jsonutil"
	"github.com/clawbake/clawbake/internal/k8s"
)

func (h *Handler) ListInstances(c echo.Context) error {
	user := auth.UserFromContext(c.Request().Context())

	instances, err := k8s.ListInstances(c.Request().Context(), h.K8s, h.Config.KubeNamespace)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list instances")
	}

	// Non-admin users only see their own instances
	if user.Role != "admin" {
		userID, _ := user.ID.Value()
		uid, _ := userID.(string)
		var filtered []v1alpha1.ClawInstance
		for _, inst := range instances {
			if inst.Spec.UserId == uid {
				filtered = append(filtered, inst)
			}
		}
		instances = filtered
	}

	return c.JSON(http.StatusOK, instances)
}

type createInstanceRequest struct {
	GatewayConfigOverride string            `json:"gatewayConfigOverride"`
	PlaceholderValues     map[string]string  `json:"placeholderValues"`
}

func (h *Handler) CreateInstance(c echo.Context) error {
	user := auth.UserFromContext(c.Request().Context())
	userID, _ := user.ID.Value()
	uid, _ := userID.(string)

	// Check if instance already exists
	if _, err := k8s.GetInstance(c.Request().Context(), h.K8s, h.Config.KubeNamespace, uid); err == nil {
		return echo.NewHTTPError(http.StatusConflict, "instance already exists")
	}

	var req createInstanceRequest
	if c.Request().ContentLength > 0 {
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
	}

	defaults, err := h.DB.GetDefaults(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load defaults")
	}

	gatewayConfig := defaults.GatewayConfig

	// Substitute template placeholders if present
	placeholders := jsonutil.ExtractPlaceholders(gatewayConfig)
	if len(placeholders) > 0 {
		var missing []string
		for _, name := range placeholders {
			if _, ok := req.PlaceholderValues[name]; !ok {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "missing required placeholder values: "+strings.Join(missing, ", "))
		}
		substituted, err := jsonutil.SubstitutePlaceholders(gatewayConfig, req.PlaceholderValues)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "failed to substitute placeholders: "+err.Error())
		}
		if !json.Valid([]byte(substituted)) {
			return echo.NewHTTPError(http.StatusBadRequest, "gateway config is not valid JSON after substitution")
		}
		gatewayConfig = substituted
	}

	// Apply optional JSON override on top
	if req.GatewayConfigOverride != "" {
		if !json.Valid([]byte(req.GatewayConfigOverride)) {
			return echo.NewHTTPError(http.StatusBadRequest, "gateway config override is not valid JSON")
		}
		merged, err := jsonutil.MergeJSON(gatewayConfig, req.GatewayConfigOverride)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "failed to merge gateway config: "+err.Error())
		}
		gatewayConfig = merged
	}

	instance := &v1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uid,
			Namespace: h.Config.KubeNamespace,
		},
		Spec: v1alpha1.ClawInstanceSpec{
			UserId:        uid,
			Image:         defaults.Image,
			GatewayToken:  generateToken(),
			GatewayConfig: gatewayConfig,
			Resources: v1alpha1.ClawInstanceResources{
				Requests: v1alpha1.ResourceList{
					CPU:    defaults.CpuRequest,
					Memory: defaults.MemoryRequest,
				},
				Limits: v1alpha1.ResourceList{
					CPU:    defaults.CpuLimit,
					Memory: defaults.MemoryLimit,
				},
			},
			Storage: v1alpha1.ClawInstanceStorage{
				Size: defaults.StorageSize,
			},
		},
	}

	if err := k8s.CreateInstance(c.Request().Context(), h.K8s, instance); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return echo.NewHTTPError(http.StatusConflict, "instance already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create instance")
	}

	return c.JSON(http.StatusCreated, instance)
}

func (h *Handler) GetInstance(c echo.Context) error {
	id := c.Param("id")

	instance, err := k8s.GetInstance(c.Request().Context(), h.K8s, h.Config.KubeNamespace, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "instance not found")
	}

	user := auth.UserFromContext(c.Request().Context())
	if user.Role != "admin" {
		userID, _ := user.ID.Value()
		uid, _ := userID.(string)
		if instance.Spec.UserId != uid {
			return echo.NewHTTPError(http.StatusNotFound, "instance not found")
		}
	}

	return c.JSON(http.StatusOK, instance)
}

func (h *Handler) DeleteInstance(c echo.Context) error {
	id := c.Param("id")

	instance, err := k8s.GetInstance(c.Request().Context(), h.K8s, h.Config.KubeNamespace, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "instance not found")
	}

	user := auth.UserFromContext(c.Request().Context())
	if user.Role != "admin" {
		userID, _ := user.ID.Value()
		uid, _ := userID.(string)
		if instance.Spec.UserId != uid {
			return echo.NewHTTPError(http.StatusNotFound, "instance not found")
		}
	}

	if err := k8s.DeleteInstance(c.Request().Context(), h.K8s, h.Config.KubeNamespace, id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete instance")
	}

	return c.NoContent(http.StatusNoContent)
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
