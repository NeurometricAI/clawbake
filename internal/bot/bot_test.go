package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/slack-go/slack"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clawbakev1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
)

const testSigningSecret = "test-secret-12345"

// mockSlackClient implements slackClient for testing.
type mockSlackClient struct {
	users    map[string]*slack.User
	messages []postedMessage
}

type postedMessage struct {
	Channel string
}

func (m *mockSlackClient) GetUserInfoContext(_ context.Context, userID string) (*slack.User, error) {
	u, ok := m.users[userID]
	if !ok {
		return nil, fmt.Errorf("user %s not found", userID)
	}
	return u, nil
}

func (m *mockSlackClient) PostMessageContext(_ context.Context, channelID string, _ ...slack.MsgOption) (string, string, error) {
	m.messages = append(m.messages, postedMessage{Channel: channelID})
	return channelID, "1234567890.123456", nil
}

func newTestBot(slackMock *mockSlackClient) *Bot {
	scheme := runtime.NewScheme()
	clawbakev1alpha1.AddToScheme(scheme)

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	return &Bot{
		slack:         slackMock,
		signingSecret: testSigningSecret,
		k8sClient:     k8sClient,
		namespace:     "clawbake",
		ingressDomain: "claw.example.com",
	}
}

func TestVerifySignature_Valid(t *testing.T) {
	b := &Bot{signingSecret: testSigningSecret}
	e := echo.New()

	body := `{"type":"url_verification","challenge":"test123"}`
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature := ComputeSignature(testSigningSecret, timestamp, body)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", timestamp)
	req.Header.Set("X-Slack-Signature", signature)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := b.verifySignature(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestVerifySignature_InvalidSignature(t *testing.T) {
	b := &Bot{signingSecret: testSigningSecret}
	e := echo.New()

	body := `{"type":"url_verification"}`
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", timestamp)
	req.Header.Set("X-Slack-Signature", "v0=invalidsignature")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := b.verifySignature(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestVerifySignature_OldTimestamp(t *testing.T) {
	b := &Bot{signingSecret: testSigningSecret}
	e := echo.New()

	body := `{"type":"test"}`
	oldTime := time.Now().Add(-10 * time.Minute).Unix()
	timestamp := strconv.FormatInt(oldTime, 10)
	signature := ComputeSignature(testSigningSecret, timestamp, body)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", timestamp)
	req.Header.Set("X-Slack-Signature", signature)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := b.verifySignature(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestVerifySignature_MissingHeaders(t *testing.T) {
	b := &Bot{signingSecret: testSigningSecret}
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := b.verifySignature(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleEvents_URLVerification(t *testing.T) {
	slackMock := &mockSlackClient{users: map[string]*slack.User{}}
	b := newTestBot(slackMock)
	e := echo.New()

	body := `{"type":"url_verification","challenge":"abc123xyz"}`

	req := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := b.HandleEvents(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["challenge"] != "abc123xyz" {
		t.Fatalf("expected challenge abc123xyz, got %s", resp["challenge"])
	}
}

func TestHandleCommands_Help(t *testing.T) {
	slackMock := &mockSlackClient{users: map[string]*slack.User{}}
	b := newTestBot(slackMock)
	e := echo.New()

	form := "command=%2Fclawbake&text=help&user_id=U123"
	req := httptest.NewRequest(http.MethodPost, "/commands", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := b.HandleCommands(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["response_type"] != "ephemeral" {
		t.Fatalf("expected ephemeral response, got %s", resp["response_type"])
	}
	if !strings.Contains(resp["text"], "Clawbake Bot Commands") {
		t.Fatalf("expected help text, got: %s", resp["text"])
	}
}

func TestGetUserInstance_NotFound(t *testing.T) {
	slackMock := &mockSlackClient{users: map[string]*slack.User{}}
	b := newTestBot(slackMock)

	_, err := b.getUserInstance(context.Background(), [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	if err == nil {
		t.Fatal("expected error for missing instance")
	}
}

func TestGetUserInstance_Found(t *testing.T) {
	slackMock := &mockSlackClient{users: map[string]*slack.User{}}
	b := newTestBot(slackMock)

	uid := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	uidStr := formatUUID(uid)

	instance := &clawbakev1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claw-test",
			Namespace: "clawbake",
		},
		Spec: clawbakev1alpha1.ClawInstanceSpec{
			UserId:      uidStr,
			DisplayName: "Test User",
			Image:       "openclaw:latest",
		},
	}
	if err := b.k8sClient.Create(context.Background(), instance); err != nil {
		t.Fatalf("failed to create test instance: %v", err)
	}

	found, err := b.getUserInstance(context.Background(), uid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found.Spec.UserId != uidStr {
		t.Fatalf("expected userId %s, got %s", uidStr, found.Spec.UserId)
	}
}

func TestFormatUUID(t *testing.T) {
	b := [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	result := formatUUID(b)
	if result == "" {
		t.Fatal("expected non-empty UUID string")
	}
	if !strings.Contains(result, "-") {
		t.Fatalf("expected UUID format with dashes, got: %s", result)
	}
}

func TestRespondSlack(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := respondSlack(c, "hello world"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["response_type"] != "ephemeral" {
		t.Fatalf("expected ephemeral, got %s", resp["response_type"])
	}
	if resp["text"] != "hello world" {
		t.Fatalf("expected 'hello world', got %s", resp["text"])
	}
}
