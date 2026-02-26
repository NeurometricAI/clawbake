package handler

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/clawbake/clawbake/internal/database"
	"github.com/clawbake/clawbake/internal/jsonutil"
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
	GatewayConfig string `json:"gatewayConfig"`
}

func (h *Handler) UpdateDefaults(c echo.Context) error {
	var req updateDefaultsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if placeholders := jsonutil.ExtractPlaceholders(req.GatewayConfig); len(placeholders) > 0 {
		if err := jsonutil.ValidateTemplate(req.GatewayConfig); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "gateway config template is not valid: "+err.Error())
		}
	} else if !json.Valid([]byte(req.GatewayConfig)) {
		return echo.NewHTTPError(http.StatusBadRequest, "gateway config is not valid JSON")
	}

	defaults, err := h.DB.UpdateDefaults(c.Request().Context(), database.UpdateDefaultsParams{
		Image:         req.Image,
		CpuRequest:    req.CpuRequest,
		MemoryRequest: req.MemoryRequest,
		CpuLimit:      req.CpuLimit,
		MemoryLimit:   req.MemoryLimit,
		StorageSize:   req.StorageSize,
		GatewayConfig: req.GatewayConfig,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update defaults")
	}

	return c.JSON(http.StatusOK, defaults)
}

func (h *Handler) GetDefaultGatewayConfig(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"gatewayConfig": h.Config.InstanceDefaultGatewayConfig,
	})
}
