package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
	"github.com/clawbake/clawbake/internal/auth"
	"github.com/clawbake/clawbake/internal/database"
	"github.com/clawbake/clawbake/internal/jsonutil"
	"github.com/clawbake/clawbake/internal/k8s"
	"github.com/clawbake/clawbake/web/templates"
)

func render(c echo.Context, status int, component templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(status)
	return component.Render(c.Request().Context(), c.Response())
}

func (h *Handler) PageDashboard(c echo.Context) error {
	user := auth.UserFromContext(c.Request().Context())
	if user == nil {
		if h.DevAuth != nil {
			return render(c, http.StatusOK, templates.DevLogin())
		}
		return render(c, http.StatusOK, templates.Login())
	}

	instances, err := k8s.ListInstances(c.Request().Context(), h.K8s, h.Config.KubeNamespace)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list instances")
	}

	userID, _ := user.ID.Value()
	uid, _ := userID.(string)

	hasInstance := false
	for _, inst := range instances {
		if inst.Spec.UserId == uid {
			hasInstance = true
			break
		}
	}

	var userNames map[string]string
	if user.Role != "admin" {
		var filtered []v1alpha1.ClawInstance
		for _, inst := range instances {
			if inst.Spec.UserId == uid {
				filtered = append(filtered, inst)
			}
		}
		instances = filtered
	} else {
		users, err := h.DB.ListUsers(c.Request().Context())
		if err == nil {
			userNames = make(map[string]string, len(users))
			for _, u := range users {
				id, _ := u.ID.Value()
				if s, ok := id.(string); ok {
					userNames[s] = u.Name
				}
			}
		}
	}

	defaults, err := h.DB.GetDefaults(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load defaults")
	}
	placeholders := jsonutil.ExtractPlaceholders(defaults.GatewayConfig)

	return render(c, http.StatusOK, templates.Dashboard(instances, user.Role == "admin", hasInstance, userNames, placeholders, h.Config.TUIEnabled, h.Config.ShellEnabled, uid))
}

func (h *Handler) PageCreateInstance(c echo.Context) error {
	user := auth.UserFromContext(c.Request().Context())
	userID, _ := user.ID.Value()
	uid, _ := userID.(string)

	// Check if already exists
	if _, err := k8s.GetInstance(c.Request().Context(), h.K8s, h.Config.KubeNamespace, uid); err == nil {
		return c.Redirect(http.StatusSeeOther, "/")
	}

	defaults, err := h.DB.GetDefaults(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load defaults")
	}

	gatewayConfig := defaults.GatewayConfig

	// Substitute template placeholders if present
	placeholders := jsonutil.ExtractPlaceholders(gatewayConfig)
	if len(placeholders) > 0 {
		values := make(map[string]string, len(placeholders))
		var missing []string
		for _, name := range placeholders {
			val := c.FormValue("var_" + name)
			if val == "" {
				missing = append(missing, name)
			} else {
				values[name] = val
			}
		}
		if len(missing) > 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "missing required values: "+strings.Join(missing, ", "))
		}
		substituted, err := jsonutil.SubstitutePlaceholders(gatewayConfig, values)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "failed to substitute placeholders: "+err.Error())
		}
		if !json.Valid([]byte(substituted)) {
			return echo.NewHTTPError(http.StatusBadRequest, "gateway config is not valid JSON after substitution")
		}
		gatewayConfig = substituted
	}

	// Apply optional JSON override on top
	if override := c.FormValue("gatewayConfigOverride"); override != "" {
		if !json.Valid([]byte(override)) {
			return echo.NewHTTPError(http.StatusBadRequest, "gateway config override is not valid JSON")
		}
		merged, err := jsonutil.MergeJSON(gatewayConfig, override)
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
			return c.Redirect(http.StatusSeeOther, "/")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create instance")
	}

	c.Response().Header().Set("HX-Redirect", "/")
	return c.NoContent(http.StatusCreated)
}

func (h *Handler) PageInstanceDetail(c echo.Context) error {
	id := c.Param("id")

	instance, err := k8s.GetInstance(c.Request().Context(), h.K8s, h.Config.KubeNamespace, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "instance not found")
	}

	user := auth.UserFromContext(c.Request().Context())
	userID, _ := user.ID.Value()
	uid, _ := userID.(string)
	if user.Role != "admin" {
		if instance.Spec.UserId != uid {
			return echo.NewHTTPError(http.StatusNotFound, "instance not found")
		}
	}

	return render(c, http.StatusOK, templates.InstanceDetail(*instance, user.Role == "admin", h.Config.TUIEnabled, h.Config.ShellEnabled, instance.Spec.UserId == uid))
}

func (h *Handler) PageInstanceStatus(c echo.Context) error {
	id := c.Param("id")

	instance, err := k8s.GetInstance(c.Request().Context(), h.K8s, h.Config.KubeNamespace, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "instance not found")
	}

	user := auth.UserFromContext(c.Request().Context())
	userID, _ := user.ID.Value()
	uid, _ := userID.(string)
	if user.Role != "admin" {
		if instance.Spec.UserId != uid {
			return echo.NewHTTPError(http.StatusNotFound, "instance not found")
		}
	}

	isOwner := instance.Spec.UserId == uid
	detail := strings.Contains(c.Request().Header.Get("HX-Current-URL"), "/ui/instances/")
	return render(c, http.StatusOK, templates.StatusPollResponse(*instance, h.Config.TUIEnabled, h.Config.ShellEnabled, isOwner, detail))
}

func (h *Handler) PageDeleteInstance(c echo.Context) error {
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

	if c.QueryParam("redirect") == "true" {
		c.Response().Header().Set("HX-Redirect", "/")
	}
	return c.NoContent(http.StatusOK)
}

func (h *Handler) PageAdminUsers(c echo.Context) error {
	user := auth.UserFromContext(c.Request().Context())
	if user.Role != "admin" {
		return c.Redirect(http.StatusFound, "/")
	}

	users, err := h.DB.ListUsers(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list users")
	}
	return render(c, http.StatusOK, templates.AdminUsers(users, true))
}

func (h *Handler) PageAdminDefaults(c echo.Context) error {
	user := auth.UserFromContext(c.Request().Context())
	if user.Role != "admin" {
		return c.Redirect(http.StatusFound, "/")
	}

	defaults, err := h.DB.GetDefaults(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get defaults")
	}
	defaults.GatewayConfig = prettyJSON(defaults.GatewayConfig)
	return render(c, http.StatusOK, templates.AdminDefaults(defaults, true))
}

func (h *Handler) PageUpdateDefaults(c echo.Context) error {
	user := auth.UserFromContext(c.Request().Context())
	if user.Role != "admin" {
		return c.Redirect(http.StatusFound, "/")
	}

	gatewayConfig := c.FormValue("gatewayConfig")
	if placeholders := jsonutil.ExtractPlaceholders(gatewayConfig); len(placeholders) > 0 {
		if err := jsonutil.ValidateTemplate(gatewayConfig); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "gateway config template is not valid: "+err.Error())
		}
	} else if !json.Valid([]byte(gatewayConfig)) {
		return echo.NewHTTPError(http.StatusBadRequest, "gateway config is not valid JSON")
	}
	// Compact the JSON for storage (only if no placeholders, since compacting breaks templates)
	if len(jsonutil.ExtractPlaceholders(gatewayConfig)) == 0 {
		gatewayConfig = compactJSON(gatewayConfig)
	}

	_, err := h.DB.UpdateDefaults(c.Request().Context(), database.UpdateDefaultsParams{
		Image:         c.FormValue("image"),
		CpuRequest:    c.FormValue("cpuRequest"),
		MemoryRequest: c.FormValue("memoryRequest"),
		CpuLimit:      c.FormValue("cpuLimit"),
		MemoryLimit:   c.FormValue("memoryLimit"),
		StorageSize:   c.FormValue("storageSize"),
		GatewayConfig: gatewayConfig,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update defaults")
	}

	return c.Redirect(http.StatusSeeOther, "/ui/admin/defaults")
}

func (h *Handler) PageEditInstanceConfig(c echo.Context) error {
	id := c.Param("id")

	instance, err := k8s.GetInstance(c.Request().Context(), h.K8s, h.Config.KubeNamespace, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "instance not found")
	}

	user := auth.UserFromContext(c.Request().Context())
	userID, _ := user.ID.Value()
	uid, _ := userID.(string)
	if user.Role != "admin" && instance.Spec.UserId != uid {
		return echo.NewHTTPError(http.StatusNotFound, "instance not found")
	}

	// Try to read the live config from the running pod
	instanceNS := "clawbake-" + instance.Name
	configJSON, err := k8s.ReadInstanceConfig(c.Request().Context(), h.K8sConfig, h.K8s, instanceNS)
	fromPod := err == nil
	if !fromPod {
		// Fall back to the CRD's gateway config
		configJSON = instance.Spec.GatewayConfig
	}

	configJSON = prettyJSON(configJSON)

	return render(c, http.StatusOK, templates.InstanceEditConfig(*instance, configJSON, fromPod, user.Role == "admin"))
}

func (h *Handler) PageUpdateInstanceConfig(c echo.Context) error {
	id := c.Param("id")

	instance, err := k8s.GetInstance(c.Request().Context(), h.K8s, h.Config.KubeNamespace, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "instance not found")
	}

	user := auth.UserFromContext(c.Request().Context())
	userID, _ := user.ID.Value()
	uid, _ := userID.(string)
	if user.Role != "admin" && instance.Spec.UserId != uid {
		return echo.NewHTTPError(http.StatusNotFound, "instance not found")
	}

	gatewayConfig := c.FormValue("gatewayConfig")
	if !json.Valid([]byte(gatewayConfig)) {
		return echo.NewHTTPError(http.StatusBadRequest, "gateway config is not valid JSON")
	}

	instance.Spec.GatewayConfig = compactJSON(gatewayConfig)
	instance.Spec.ConfigGeneration++

	if err := k8s.UpdateInstance(c.Request().Context(), h.K8s, instance); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update instance config")
	}

	return c.Redirect(http.StatusSeeOther, "/ui/instances/"+id)
}

func prettyJSON(s string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(s), "", "  "); err != nil {
		return s
	}
	return buf.String()
}

func compactJSON(s string) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		return s
	}
	return buf.String()
}
