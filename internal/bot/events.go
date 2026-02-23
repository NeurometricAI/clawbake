package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"

	clawbakev1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
)

// HandleEvents processes Slack Events API requests.
func (b *Bot) HandleEvents(c echo.Context) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read body"})
	}

	event, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to parse event"})
	}

	switch event.Type {
	case slackevents.URLVerification:
		var challenge struct {
			Challenge string `json:"challenge"`
		}
		if err := json.Unmarshal(body, &challenge); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to parse challenge"})
		}
		return c.JSON(http.StatusOK, map[string]string{"challenge": challenge.Challenge})

	case slackevents.CallbackEvent:
		switch ev := event.InnerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			go b.handleMessage(ev.Channel, ev.User, ev.Text)
		case *slackevents.MessageEvent:
			if ev.BotID != "" {
				break
			}
			go b.handleMessage(ev.Channel, ev.User, ev.Text)
		}
	}

	return c.NoContent(http.StatusOK)
}

func (b *Bot) handleMessage(channel, slackUserID, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	user, err := b.resolveUser(ctx, slackUserID)
	if err != nil {
		b.slack.PostMessageContext(ctx, channel, slack.MsgOptionText(
			fmt.Sprintf("Failed to look up your account: %s", err), false))
		return
	}

	instance, err := b.getUserInstance(ctx, user.ID.Bytes)
	if err != nil {
		b.slack.PostMessageContext(ctx, channel, slack.MsgOptionText(
			"You don't have an openclaw instance yet. Use `/clawbake create` to create one.", false))
		return
	}

	if instance.Status.Phase != clawbakev1alpha1.PhaseRunning {
		b.slack.PostMessageContext(ctx, channel, slack.MsgOptionText(
			fmt.Sprintf("Your instance is currently %s. Please wait for it to be Running.", instance.Status.Phase), false))
		return
	}

	response, err := b.forwardToInstance(ctx, instance, text)
	if err != nil {
		b.slack.PostMessageContext(ctx, channel, slack.MsgOptionText(
			fmt.Sprintf("Failed to forward message to your instance: %s", err), false))
		return
	}

	b.slack.PostMessageContext(ctx, channel, slack.MsgOptionText(response, false))
}
