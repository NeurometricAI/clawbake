package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
		log.Printf("slack: failed to parse event: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to parse event"})
	}

	log.Printf("slack: received event type=%s", event.Type)

	switch event.Type {
	case slackevents.URLVerification:
		log.Printf("slack: handling URL verification challenge")
		var challenge struct {
			Challenge string `json:"challenge"`
		}
		if err := json.Unmarshal(body, &challenge); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to parse challenge"})
		}
		return c.JSON(http.StatusOK, map[string]string{"challenge": challenge.Challenge})

	case slackevents.CallbackEvent:
		innerType := event.InnerEvent.Type
		switch ev := event.InnerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			log.Printf("slack: app_mention from user=%s channel=%s", ev.User, ev.Channel)
			go b.handleMessage(ev.Channel, ev.User, ev.Text)
		case *slackevents.MessageEvent:
			if ev.BotID != "" {
				log.Printf("slack: ignoring message from bot=%s", ev.BotID)
				break
			}
			log.Printf("slack: message from user=%s channel=%s", ev.User, ev.Channel)
			go b.handleMessage(ev.Channel, ev.User, ev.Text)
		default:
			log.Printf("slack: unhandled inner event type=%s", innerType)
		}
	}

	return c.NoContent(http.StatusOK)
}

func (b *Bot) handleMessage(channel, slackUserID, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	user, err := b.resolveUser(ctx, slackUserID)
	if err != nil {
		log.Printf("slack: failed to resolve user %s: %v", slackUserID, err)
		b.slack.PostMessageContext(ctx, channel, slack.MsgOptionText(
			fmt.Sprintf("Failed to look up your account: %s", err), false))
		return
	}

	instance, err := b.getUserInstance(ctx, user.ID.Bytes)
	if err != nil {
		log.Printf("slack: no instance for user %s: %v", user.Email, err)
		b.slack.PostMessageContext(ctx, channel, slack.MsgOptionText(
			"You don't have an openclaw instance yet. Use `/clawbake create` to create one.", false))
		return
	}

	if instance.Status.Phase != clawbakev1alpha1.PhaseRunning {
		log.Printf("slack: instance for user %s not running (phase=%s)", user.Email, instance.Status.Phase)
		b.slack.PostMessageContext(ctx, channel, slack.MsgOptionText(
			fmt.Sprintf("Your instance is currently %s. Please wait for it to be Running.", instance.Status.Phase), false))
		return
	}

	log.Printf("slack: forwarding message from user %s to instance %s", user.Email, instance.Name)
	response, err := b.forwardToInstance(ctx, instance, text)
	if err != nil {
		log.Printf("slack: failed to forward to instance %s: %v", instance.Name, err)
		b.slack.PostMessageContext(ctx, channel, slack.MsgOptionText(
			fmt.Sprintf("Failed to forward message to your instance: %s", err), false))
		return
	}

	b.slack.PostMessageContext(ctx, channel, slack.MsgOptionText(response, false))
}
