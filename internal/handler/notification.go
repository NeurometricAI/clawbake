package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

type instanceReadyRequest struct {
	InstanceName string `json:"instanceName"`
	UserID       string `json:"userId"`
}

func (h *Handler) NotifyInstanceReady(c echo.Context) error {
	if h.Bot == nil {
		return c.NoContent(http.StatusOK)
	}

	var req instanceReadyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.UserID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "userId is required"})
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		h.Bot.NotifyInstanceReady(ctx, req.InstanceName, req.UserID)
	}()

	return c.NoContent(http.StatusOK)
}
