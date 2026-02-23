package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/clawbake/clawbake/internal/database"
)

func (h *Handler) ListUsers(c echo.Context) error {
	users, err := h.DB.ListUsers(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list users")
	}
	return c.JSON(http.StatusOK, users)
}

func (h *Handler) GetDefaults(c echo.Context) error {
	defaults, err := h.DB.GetDefaults(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get defaults")
	}
	return c.JSON(http.StatusOK, defaults)
}

type updateDefaultsRequest struct {
	Image         string `json:"image"`
	CpuRequest    string `json:"cpuRequest"`
	MemoryRequest string `json:"memoryRequest"`
	CpuLimit      string `json:"cpuLimit"`
	MemoryLimit   string `json:"memoryLimit"`
	StorageSize   string `json:"storageSize"`
	IngressDomain string `json:"ingressDomain"`
}

func (h *Handler) UpdateDefaults(c echo.Context) error {
	var req updateDefaultsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	defaults, err := h.DB.UpdateDefaults(c.Request().Context(), database.UpdateDefaultsParams{
		Image:         req.Image,
		CpuRequest:    req.CpuRequest,
		MemoryRequest: req.MemoryRequest,
		CpuLimit:      req.CpuLimit,
		MemoryLimit:   req.MemoryLimit,
		StorageSize:   req.StorageSize,
		IngressDomain: req.IngressDomain,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update defaults")
	}

	return c.JSON(http.StatusOK, defaults)
}
