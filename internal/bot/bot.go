package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/slack-go/slack"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clawbakev1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
	"github.com/clawbake/clawbake/internal/database"
)

// slackClient defines the Slack API methods used by the bot.
type slackClient interface {
	GetUserInfoContext(ctx context.Context, user string) (*slack.User, error)
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
}

// Bot handles Slack interactions and bridges them to clawbake.
type Bot struct {
	slack         slackClient
	signingSecret string
	db            *database.Queries
	k8sClient     client.Client
	namespace     string
	baseURL       string
	ttydEnabled   bool
}

// New creates a Bot with the given configuration.
func New(botToken, signingSecret string, db *database.Queries, k8sClient client.Client, namespace, baseURL string, ttydEnabled bool) *Bot {
	return &Bot{
		slack:         slack.New(botToken),
		signingSecret: signingSecret,
		db:            db,
		k8sClient:     k8sClient,
		namespace:     namespace,
		baseURL:       baseURL,
		ttydEnabled:   ttydEnabled,
	}
}

// resolveUser maps a Slack user ID to a clawbake user, creating one if needed.
func (b *Bot) resolveUser(ctx context.Context, slackUserID string) (database.User, error) {
	slackUser, err := b.slack.GetUserInfoContext(ctx, slackUserID)
	if err != nil {
		return database.User{}, fmt.Errorf("getting Slack user info: %w", err)
	}

	email := slackUser.Profile.Email
	if email == "" {
		return database.User{}, fmt.Errorf("Slack user has no email configured")
	}

	user, err := b.db.GetUserByEmail(ctx, email)
	if err == nil {
		if !user.SlackUserID.Valid {
			user, _ = b.db.UpdateUser(ctx, database.UpdateUserParams{
				ID:          user.ID,
				SlackUserID: pgtype.Text{String: slackUserID, Valid: true},
			})
		}
		return user, nil
	}

	return b.db.CreateUser(ctx, database.CreateUserParams{
		Email:       email,
		Name:        slackUser.Profile.RealName,
		Role:        "user",
		OidcSubject: fmt.Sprintf("slack:%s", slackUserID),
		SlackUserID: pgtype.Text{String: slackUserID, Valid: true},
	})
}

// getUserInstance finds the ClawInstance for a given user UUID.
func (b *Bot) getUserInstance(ctx context.Context, userID [16]byte) (*clawbakev1alpha1.ClawInstance, error) {
	uid := formatUUID(userID)
	var list clawbakev1alpha1.ClawInstanceList
	if err := b.k8sClient.List(ctx, &list, client.InNamespace(b.namespace)); err != nil {
		return nil, fmt.Errorf("listing instances: %w", err)
	}
	for i := range list.Items {
		if list.Items[i].Spec.UserId == uid {
			return &list.Items[i], nil
		}
	}
	return nil, fmt.Errorf("no instance found for user %s", uid)
}

// forwardToInstance sends a message to the user's openclaw instance via the OpenAI-compatible
// chat completions endpoint and returns the response.
func (b *Bot) forwardToInstance(ctx context.Context, instance *clawbakev1alpha1.ClawInstance, message string) (string, error) {
	ns := instance.Status.Namespace
	if ns == "" {
		ns = fmt.Sprintf("clawbake-%s", instance.Spec.UserId)
	}
	url := fmt.Sprintf("http://openclaw.%s.svc.cluster.local:18789/v1/chat/completions", ns)

	type chatMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	payload, err := json.Marshal(map[string]any{
		"model":    "openclaw",
		"messages": []chatMessage{{Role: "user", Content: message}},
	})
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	httpClient := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+instance.Spec.GatewayToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("forwarding to instance: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("instance returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse OpenAI-compatible response
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return string(body), nil
	}
	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}
	return string(body), nil
}

// NotifyInstanceReady sends a Slack DM to the user when their instance becomes ready.
// It's a no-op for non-Slack users or if the user can't be found.
func (b *Bot) NotifyInstanceReady(ctx context.Context, instanceName, userID string) {
	parsedID, err := uuid.Parse(userID)
	if err != nil {
		return
	}

	user, err := b.db.GetUserByID(ctx, pgtype.UUID{Bytes: parsedID, Valid: true})
	if err != nil {
		return
	}

	if !user.SlackUserID.Valid {
		return
	}
	slackUserID := user.SlackUserID.String

	msg := fmt.Sprintf(
		"Your openclaw instance *%s* is now running! :tada:\n\n"+
			"You can interact with it by:\n"+
			"- Sending me a DM with your message\n"+
			"- @mentioning me in a channel\n"+
			"- Using the web UI at %s",
		instanceName, b.baseURL,
	)

	_, _, _ = b.slack.PostMessageContext(ctx, slackUserID, slack.MsgOptionText(msg, false))
}

func formatUUID(b [16]byte) string {
	u, err := uuid.FromBytes(b[:])
	if err != nil {
		return fmt.Sprintf("%x", b)
	}
	return u.String()
}
