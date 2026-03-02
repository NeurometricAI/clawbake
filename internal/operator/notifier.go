package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// HTTPNotifier sends instance-ready notifications to the clawbake server via HTTP.
type HTTPNotifier struct {
	ServerURL string
	Client    *http.Client
}

// NewHTTPNotifier creates an HTTPNotifier that posts to the given server URL.
func NewHTTPNotifier(serverURL string) *HTTPNotifier {
	return &HTTPNotifier{
		ServerURL: serverURL,
		Client:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (n *HTTPNotifier) NotifyInstanceReady(ctx context.Context, instanceName, userID string) {
	logger := log.FromContext(ctx)

	payload, err := json.Marshal(map[string]string{
		"instanceName": instanceName,
		"userId":       userID,
	})
	if err != nil {
		logger.Error(err, "failed to marshal notification payload")
		return
	}

	url := fmt.Sprintf("%s/internal/notifications/instance-ready", n.ServerURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		logger.Error(err, "failed to create notification request")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.Client.Do(req)
	if err != nil {
		logger.Error(err, "failed to send instance-ready notification")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Info("instance-ready notification returned non-200", "status", resp.StatusCode)
	}
}
