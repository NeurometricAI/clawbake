package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
	"github.com/clawbake/clawbake/internal/auth"
	"github.com/clawbake/clawbake/internal/k8s"
)

type createInstanceRequest struct {
	DisplayName string `json:"displayName"`
	Image       string `json:"image,omitempty"`
}

type updateInstanceRequest struct {
	DisplayName string `json:"displayName,omitempty"`
	Image       string `json:"image,omitempty"`
}

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

func (h *Handler) CreateInstance(c echo.Context) error {
	var req createInstanceRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.DisplayName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "displayName is required")
	}

	user := auth.UserFromContext(c.Request().Context())
	userID, _ := user.ID.Value()
	uid, _ := userID.(string)

	instances, err := k8s.ListInstances(c.Request().Context(), h.K8s, h.Config.KubeNamespace)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list instances")
	}
	if countUserInstances(instances, uid) >= int(user.InstanceLimit) {
		return echo.NewHTTPError(http.StatusConflict, "instance limit reached")
	}

	name := sanitizeName(req.DisplayName)
	if instanceNameExists(instances, name) {
		return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("name %q is already taken", req.DisplayName))
	}

	// Load defaults from DB
	defaults, err := h.DB.GetDefaults(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load defaults")
	}

	image := defaults.Image
	if req.Image != "" {
		image = req.Image
	}

	host := fmt.Sprintf("%s.%s", name, defaults.IngressDomain)

	instance := &v1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: h.Config.KubeNamespace,
		},
		Spec: v1alpha1.ClawInstanceSpec{
			UserId:       uid,
			DisplayName:  req.DisplayName,
			Image:        image,
			GatewayToken: generateToken(),
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
			Ingress: v1alpha1.ClawInstanceIngress{
				Enabled: true,
				Host:    host,
			},
		},
	}

	if err := k8s.CreateInstance(c.Request().Context(), h.K8s, instance); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("name %q is already taken", req.DisplayName))
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

func (h *Handler) UpdateInstance(c echo.Context) error {
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

	var req updateInstanceRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.DisplayName != "" {
		instance.Spec.DisplayName = req.DisplayName
	}
	if req.Image != "" {
		instance.Spec.Image = req.Image
	}

	if err := h.K8s.Update(c.Request().Context(), instance); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update instance")
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

func sanitizeName(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	// Keep only alphanumeric and hyphens
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	// Trim leading/trailing hyphens and limit length
	result = strings.Trim(result, "-")
	if len(result) > 63 {
		result = result[:63]
	}
	if result == "" {
		result = "instance"
	}
	return result
}
