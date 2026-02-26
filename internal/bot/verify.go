package bot

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// verifySignature is Echo middleware that validates Slack request signatures.
func (b *Bot) verifySignature(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		log.Printf("slack: incoming %s %s", c.Request().Method, c.Request().URL.Path)

		timestamp := c.Request().Header.Get("X-Slack-Request-Timestamp")
		signature := c.Request().Header.Get("X-Slack-Signature")

		if timestamp == "" || signature == "" {
			log.Printf("slack: rejected request: missing signature headers")
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing signature headers"})
		}

		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			log.Printf("slack: rejected request: invalid timestamp %q", timestamp)
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid timestamp"})
		}

		age := time.Now().Unix() - ts
		if math.Abs(float64(age)) > 300 {
			log.Printf("slack: rejected request: timestamp too old (age=%ds)", age)
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
			log.Printf("slack: rejected request: invalid signature")
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
