package bot

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/slack-go/slack"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clawbakev1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
)

// HandleCommands processes Slack slash commands (/clawbake).
func (b *Bot) HandleCommands(c echo.Context) error {
	cmd, err := slack.SlashCommandParse(c.Request())
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to parse command"})
	}

	parts := strings.Fields(cmd.Text)
	if len(parts) == 0 {
		parts = []string{"help"}
	}

	ctx := c.Request().Context()
	switch parts[0] {
	case "create":
		return b.handleCreate(ctx, c, cmd)
	case "status":
		return b.handleStatus(ctx, c, cmd)
	case "delete":
		return b.handleDelete(ctx, c, cmd)
	default:
		return b.handleHelp(c)
	}
}

func (b *Bot) handleCreate(ctx context.Context, c echo.Context, cmd slack.SlashCommand) error {
	user, err := b.resolveUser(ctx, cmd.UserID)
	if err != nil {
		return respondSlack(c, fmt.Sprintf("Failed to look up your account: %s", err))
	}

	uid := formatUUID(user.ID.Bytes)

	if existing, _ := b.getUserInstance(ctx, user.ID.Bytes); existing != nil {
		return respondSlack(c, fmt.Sprintf("You already have an instance (status: %s).", existing.Status.Phase))
	}

	defaults, err := b.db.GetDefaults(ctx)
	if err != nil {
		return respondSlack(c, "Failed to get instance defaults. Please contact an admin.")
	}

	instance := &clawbakev1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("claw-%s", uid),
			Namespace: b.namespace,
			Labels: map[string]string{
				"clawbake.io/user-id": uid,
				"clawbake.io/source":  "slack-bot",
			},
		},
		Spec: clawbakev1alpha1.ClawInstanceSpec{
			UserId:      uid,
			DisplayName: user.Name,
			Image:       defaults.Image,
			Resources: clawbakev1alpha1.ClawInstanceResources{
				Requests: clawbakev1alpha1.ResourceList{
					CPU:    defaults.CpuRequest,
					Memory: defaults.MemoryRequest,
				},
				Limits: clawbakev1alpha1.ResourceList{
					CPU:    defaults.CpuLimit,
					Memory: defaults.MemoryLimit,
				},
			},
			Storage: clawbakev1alpha1.ClawInstanceStorage{
				Size: defaults.StorageSize,
			},
			Ingress: clawbakev1alpha1.ClawInstanceIngress{
				Enabled: true,
				Host:    fmt.Sprintf("%s.%s", uid, b.ingressDomain),
			},
		},
	}

	if err := b.k8sClient.Create(ctx, instance); err != nil {
		return respondSlack(c, fmt.Sprintf("Failed to create instance: %s", err))
	}

	return respondSlack(c, fmt.Sprintf(
		"Creating your openclaw instance! It will be available at https://%s.%s once ready.\nUse `/clawbake status` to check progress.",
		uid, b.ingressDomain))
}

func (b *Bot) handleStatus(ctx context.Context, c echo.Context, cmd slack.SlashCommand) error {
	user, err := b.resolveUser(ctx, cmd.UserID)
	if err != nil {
		return respondSlack(c, fmt.Sprintf("Failed to look up your account: %s", err))
	}

	instance, err := b.getUserInstance(ctx, user.ID.Bytes)
	if err != nil {
		return respondSlack(c, "You don't have an openclaw instance. Use `/clawbake create` to create one.")
	}

	msg := fmt.Sprintf("*Instance Status*\n• Phase: %s\n• Namespace: %s",
		instance.Status.Phase, instance.Status.Namespace)
	if instance.Status.URL != "" {
		msg += fmt.Sprintf("\n• URL: %s", instance.Status.URL)
	}
	return respondSlack(c, msg)
}

func (b *Bot) handleDelete(ctx context.Context, c echo.Context, cmd slack.SlashCommand) error {
	user, err := b.resolveUser(ctx, cmd.UserID)
	if err != nil {
		return respondSlack(c, fmt.Sprintf("Failed to look up your account: %s", err))
	}

	instance, err := b.getUserInstance(ctx, user.ID.Bytes)
	if err != nil {
		return respondSlack(c, "You don't have an openclaw instance.")
	}

	if err := b.k8sClient.Delete(ctx, instance); err != nil {
		return respondSlack(c, fmt.Sprintf("Failed to delete instance: %s", err))
	}

	return respondSlack(c, "Your openclaw instance is being deleted.")
}

func (b *Bot) handleHelp(c echo.Context) error {
	return respondSlack(c, "*Clawbake Bot Commands*\n"+
		"• `/clawbake create` - Provision a new openclaw instance\n"+
		"• `/clawbake status` - Show your instance status\n"+
		"• `/clawbake delete` - Delete your instance\n"+
		"• `/clawbake help` - Show this help message")
}

func respondSlack(c echo.Context, text string) error {
	return c.JSON(http.StatusOK, map[string]string{
		"response_type": "ephemeral",
		"text":          text,
	})
}
