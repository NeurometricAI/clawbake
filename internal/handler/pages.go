package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
	"github.com/clawbake/clawbake/internal/auth"
	"github.com/clawbake/clawbake/internal/database"
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
	userInstanceCount := countUserInstances(instances, uid)
	atLimit := userInstanceCount >= int(user.InstanceLimit)

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

	return render(c, http.StatusOK, templates.Dashboard(instances, user.Role == "admin", atLimit, userNames))
}

func countUserInstances(instances []v1alpha1.ClawInstance, uid string) int {
	count := 0
	for _, inst := range instances {
		if inst.Spec.UserId == uid {
			count++
		}
	}
	return count
}

func instanceNameExists(instances []v1alpha1.ClawInstance, name string) bool {
	for _, inst := range instances {
		if inst.Name == name {
			return true
		}
	}
	return false
}

func (h *Handler) PageCreateForm(c echo.Context) error {
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

	return render(c, http.StatusOK, templates.CreateForm())
}

func (h *Handler) PageCreateInstance(c echo.Context) error {
	displayName := c.FormValue("displayName")
	if displayName == "" {
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
		return render(c, http.StatusOK, templates.CreateFormWithError("Instance limit reached"))
	}

	name := sanitizeName(displayName)
	if instanceNameExists(instances, name) {
		return render(c, http.StatusOK, templates.CreateFormWithError(fmt.Sprintf("Name %q is already taken, choose a different name", displayName)))
	}

	defaults, err := h.DB.GetDefaults(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load defaults")
	}

	image := defaults.Image
	if img := c.FormValue("image"); img != "" {
		image = img
	}

	host := fmt.Sprintf("%s.%s", name, defaults.IngressDomain)

	instance := &v1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: h.Config.KubeNamespace,
		},
		Spec: v1alpha1.ClawInstanceSpec{
			UserId:       uid,
			DisplayName:  displayName,
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
			return render(c, http.StatusOK, templates.CreateFormWithError(fmt.Sprintf("Name %q is already taken, choose a different name", displayName)))
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create instance")
	}

	// +1 because we just created one
	atLimit := countUserInstances(instances, uid)+1 >= int(user.InstanceLimit)
	return render(c, http.StatusCreated, templates.InstanceCreated(*instance, atLimit))
}

func (h *Handler) PageInstanceDetail(c echo.Context) error {
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

	return render(c, http.StatusOK, templates.InstanceDetail(*instance, user.Role == "admin"))
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
	return render(c, http.StatusOK, templates.AdminDefaults(defaults, true))
}

func (h *Handler) PageUpdateDefaults(c echo.Context) error {
	user := auth.UserFromContext(c.Request().Context())
	if user.Role != "admin" {
		return c.Redirect(http.StatusFound, "/")
	}

	_, err := h.DB.UpdateDefaults(c.Request().Context(), database.UpdateDefaultsParams{
		Image:         c.FormValue("image"),
		CpuRequest:    c.FormValue("cpuRequest"),
		MemoryRequest: c.FormValue("memoryRequest"),
		CpuLimit:      c.FormValue("cpuLimit"),
		MemoryLimit:   c.FormValue("memoryLimit"),
		StorageSize:   c.FormValue("storageSize"),
		IngressDomain: c.FormValue("ingressDomain"),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update defaults")
	}

	return c.Redirect(http.StatusSeeOther, "/ui/admin/defaults")
}

func (h *Handler) PageUpdateUserLimit(c echo.Context) error {
	user := auth.UserFromContext(c.Request().Context())
	if user.Role != "admin" {
		return c.Redirect(http.StatusFound, "/")
	}

	var uid pgtype.UUID
	if err := uid.Scan(c.Param("id")); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	limit, err := strconv.Atoi(c.FormValue("instanceLimit"))
	if err != nil || limit < 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid instance limit")
	}

	updated, err := h.DB.UpdateUser(c.Request().Context(), database.UpdateUserParams{
		ID:            uid,
		InstanceLimit: pgtype.Int4{Int32: int32(limit), Valid: true},
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update user")
	}

	return render(c, http.StatusOK, templates.AdminUserRow(updated))
}
