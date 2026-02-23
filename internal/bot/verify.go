package bot

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// verifySignature is Echo middleware that validates Slack request signatures.
func (b *Bot) verifySignature(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		timestamp := c.Request().Header.Get("X-Slack-Request-Timestamp")
		signature := c.Request().Header.Get("X-Slack-Signature")

		if timestamp == "" || signature == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing signature headers"})
		}

		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid timestamp"})
		}

		if math.Abs(float64(time.Now().Unix()-ts)) > 300 {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "request too old"})
		}

		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read body"})
		}
		c.Request().Body = io.NopCloser(bytes.NewReader(body))

		baseString := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
		mac := hmac.New(sha256.New, []byte(b.signingSecret))
		mac.Write([]byte(baseString))
		expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

		if !hmac.Equal([]byte(expected), []byte(signature)) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		}

		return next(c)
	}
}

// ComputeSignature computes a Slack request signature for testing.
func ComputeSignature(signingSecret, timestamp, body string) string {
	baseString := fmt.Sprintf("v0:%s:%s", timestamp, body)
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(baseString))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}
