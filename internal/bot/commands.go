package bot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/slack-go/slack"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clawbakev1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
	"github.com/clawbake/clawbake/internal/database"
	"github.com/clawbake/clawbake/internal/jsonutil"
)

// HandleCommands processes Slack slash commands (/clawbake).
func (b *Bot) HandleCommands(c echo.Context) error {
	cmd, err := slack.SlashCommandParse(c.Request())
	if err != nil {
		log.Printf("slack: failed to parse slash command: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to parse command"})
	}

	parts := strings.Fields(cmd.Text)
	if len(parts) == 0 {
		parts = []string{"help"}
	}

	log.Printf("slack: slash command %q from user=%s", cmd.Text, cmd.UserID)

	ctx := c.Request().Context()
	switch parts[0] {
	case "create":
		return b.handleCreate(ctx, c, cmd)
	case "status":
		return b.handleStatus(ctx, c, cmd)
	case "delete":
		return b.handleDelete(ctx, c, cmd)
	case "open":
		return b.handleOpen(ctx, c, cmd)
	default:
		return b.handleHelp(ctx, c)
	}
}

// parseCreateArgs parses the arguments after "create" in a slash command.
// It supports KEY=value pairs for placeholders and json={...} for override.
// Bare {...} still works as override when no KEY=value pairs are present.
func parseCreateArgs(text string) (placeholderValues map[string]string, jsonOverride string, err error) {
	args := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "create"))
	if args == "" {
		return nil, "", nil
	}

	// Check for json= prefix to extract override
	placeholderValues = make(map[string]string)
	var remaining []string

	parts := strings.Fields(args)
	for i := 0; i < len(parts); i++ {
		part := parts[i]

		// Handle json={...} — everything from "json=" onward is the JSON override
		if strings.HasPrefix(part, "json=") {
			jsonOverride = strings.TrimPrefix(part, "json=")
			// If the JSON contains spaces, rejoin the rest
			if i+1 < len(parts) {
				jsonOverride = jsonOverride + " " + strings.Join(parts[i+1:], " ")
			}
			break
		}

		// Handle KEY=value pairs (split on first = so values can contain =)
		if idx := strings.Index(part, "="); idx > 0 {
			key := part[:idx]
			value := part[idx+1:]
			placeholderValues[key] = value
		} else {
			remaining = append(remaining, part)
			// If this starts with {, treat rest as bare JSON override (backwards compat)
			if strings.HasPrefix(part, "{") {
				jsonOverride = strings.Join(append([]string{part}, parts[i+1:]...), " ")
				break
			}
		}
	}

	_ = remaining
	if len(placeholderValues) == 0 {
		placeholderValues = nil
	}

	return placeholderValues, jsonOverride, nil
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

	placeholderValues, jsonOverride, err := parseCreateArgs(cmd.Text)
	if err != nil {
		return respondSlack(c, fmt.Sprintf("Invalid arguments: %s", err))
	}

	gatewayConfig := defaults.GatewayConfig

	// Substitute template placeholders if present
	placeholders := jsonutil.ExtractPlaceholders(gatewayConfig)
	if len(placeholders) > 0 {
		var missing []string
		for _, name := range placeholders {
			if _, ok := placeholderValues[name]; !ok {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			usage := "Usage: `/clawbake create"
			for _, p := range placeholders {
				usage += fmt.Sprintf(" %s=value", p)
			}
			usage += "`"
			return respondSlack(c, fmt.Sprintf("Missing required values: %s\n%s", strings.Join(missing, ", "), usage))
		}
		substituted, err := jsonutil.SubstitutePlaceholders(gatewayConfig, placeholderValues)
		if err != nil {
			return respondSlack(c, fmt.Sprintf("Failed to substitute placeholders: %s", err))
		}
		if !json.Valid([]byte(substituted)) {
			return respondSlack(c, "Gateway config is not valid JSON after substitution.")
		}
		gatewayConfig = substituted
	}

	// Apply optional JSON override on top
	if jsonOverride != "" {
		if !json.Valid([]byte(jsonOverride)) {
			return respondSlack(c, "Invalid JSON override. Usage: `/clawbake create json={\"key\":\"value\"}`")
		}
		merged, err := jsonutil.MergeJSON(gatewayConfig, jsonOverride)
		if err != nil {
			return respondSlack(c, fmt.Sprintf("Failed to merge config override: %s", err))
		}
		gatewayConfig = merged
	}

	instance := &clawbakev1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      uid,
			Namespace: b.namespace,
			Labels: map[string]string{
				"clawbake.io/user-id": uid,
				"clawbake.io/source":  "slack-bot",
			},
		},
		Spec: clawbakev1alpha1.ClawInstanceSpec{
			UserId:        uid,
			Image:         defaults.Image,
			GatewayToken:  generateToken(),
			GatewayConfig: gatewayConfig,
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
		},
	}

	if err := b.k8sClient.Create(ctx, instance); err != nil {
		return respondSlack(c, fmt.Sprintf("Failed to create instance: %s", err))
	}

	return respondSlack(c, "Creating your openclaw instance! Access it via the web app once ready.\nUse `/clawbake status` to check progress.")
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

func (b *Bot) handleOpen(ctx context.Context, c echo.Context, cmd slack.SlashCommand) error {
	user, err := b.resolveUser(ctx, cmd.UserID)
	if err != nil {
		return respondSlack(c, fmt.Sprintf("Failed to look up your account: %s", err))
	}

	instance, err := b.getUserInstance(ctx, user.ID.Bytes)
	if err != nil {
		return respondSlack(c, "You don't have an openclaw instance. Use `/clawbake create` to create one.")
	}

	if instance.Status.Phase != "Running" {
		return respondSlack(c, fmt.Sprintf("Your instance isn't ready yet (status: %s). Try again shortly.", instance.Status.Phase))
	}

	return respondSlack(c, fmt.Sprintf("Open your dashboard: %s/proxy/", b.baseURL))
}

func (b *Bot) handleHelp(ctx context.Context, c echo.Context) error {
	createUsage := "• `/clawbake create` - Provision a new openclaw instance\n" +
		"• `/clawbake create {\"key\":\"value\"}` - Create with gateway config overrides (merged over admin defaults)"

	// Show dynamic usage if admin config has placeholders
	defaults, err := func() (*database.InstanceDefault, error) {
		if b.db == nil {
			return nil, fmt.Errorf("no database")
		}
		d, err := b.db.GetDefaults(ctx)
		return &d, err
	}()
	if err == nil {
		placeholders := jsonutil.ExtractPlaceholders(defaults.GatewayConfig)
		if len(placeholders) > 0 {
			example := "• `/clawbake create"
			for _, p := range placeholders {
				example += fmt.Sprintf(" %s=value", p)
			}
			example += "` - Provision with required config values"
			createUsage = example + "\n" +
				"• `/clawbake create " + placeholders[0] + "=value json={\"key\":\"value\"}` - With config values and override"
		}
	}

	return respondSlack(c, "*Clawbake Bot Commands*\n"+
		createUsage+"\n"+
		"• `/clawbake status` - Show your instance status\n"+
		"• `/clawbake open` - Get a link to your instance dashboard\n"+
		"• `/clawbake delete` - Delete your instance\n"+
		"• `/clawbake help` - Show this help message")
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func respondSlack(c echo.Context, text string) error {
	return c.JSON(http.StatusOK, map[string]string{
		"response_type": "ephemeral",
		"text":          text,
	})
}
