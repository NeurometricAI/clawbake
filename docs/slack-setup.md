# Slack Bot Setup

This guide walks through creating and configuring a Slack app to connect to the clawbake bot.

## Prerequisites

- A Slack workspace where you have permission to install apps
- The clawbake server deployed and reachable via a public URL (e.g. `https://clawbake.example.com`)

## 1. Prepare the Manifest

The app manifest at [`slack-app-manifest.yaml`](../slack-app-manifest.yaml) pre-configures all scopes, events, slash commands, and bot settings. Before using it, replace the two `YOUR_DOMAIN` placeholders with your clawbake server's public domain:

```bash
sed 's/YOUR_DOMAIN/clawbake.example.com/g' slack-app-manifest.yaml
```

For local development, use a tunnel URL instead (see [Local development](#local-development) below).

## 2. Create the Slack App from the Manifest

1. Go to [api.slack.com/apps](https://api.slack.com/apps) and click **Create New App**
2. Choose **From an app manifest**
3. Select your workspace and click **Next**
4. Paste the contents of your updated `slack-app-manifest.yaml`
5. Review the summary — it should show:
   - Bot scopes: `chat:write`, `commands`, `users:read`, `users:read.email`
   - Slash command: `/clawbake`
   - Event subscriptions: `app_mention`, `message.im`
6. Click **Create**

## 3. Install the App and Collect Credentials

1. Navigate to **Install App** in the sidebar
2. Click **Install to Workspace** and authorize
3. Copy the **Bot User OAuth Token** (starts with `xoxb-`)
4. Navigate to **Basic Information** in the sidebar
5. Under **App Credentials**, copy the **Signing Secret**

## 4. Configure Clawbake

Set the two environment variables on the clawbake server:

```bash
SLACK_BOT_TOKEN=xoxb-...      # Bot User OAuth Token from step 3
SLACK_SIGNING_SECRET=...       # Signing Secret from step 3
```

### Helm deployment

In your Helm values:

```yaml
slack:
  enabled: true
  botToken: "xoxb-..."
  signingSecret: "..."
```

Or via the command line:

```bash
helm install clawbake charts/clawbake \
  --set slack.enabled=true \
  --set slack.botToken="$SLACK_BOT_TOKEN" \
  --set slack.signingSecret="$SLACK_SIGNING_SECRET"
```

### Local development

1. Start a tunnel to the **server port** (8081, not 8080 which is the k3d ingress):

   ```bash
   ngrok http 8081
   ```

2. Use the ngrok URL as `YOUR_DOMAIN` in the manifest and create the Slack app (steps 1–3 above).

3. Add the credentials to `.env.local`:

   ```
   SLACK_BOT_TOKEN=xoxb-...
   SLACK_SIGNING_SECRET=...
   ```

   Both must be set — the server skips registering `/slack/*` routes entirely when either is empty, and you'll get 404s.

4. Restart the server (`make run-server`) for the new env vars to take effect.

When the ngrok URL changes, update it in two places in the Slack app settings: **Event Subscriptions** > Request URL, and **Slash Commands** > edit the `/clawbake` command's Request URL.

## What the Manifest Configures

The manifest sets up everything the bot needs automatically:

| Setting | Value | Manual equivalent |
|---------|-------|-------------------|
| Bot display name | `clawbake` | App Home > Bot User |
| Messages tab | Enabled | App Home > Show Tabs |
| Always online | Yes | App Home > Show Tabs |
| Slash command | `/clawbake` → `/slack/commands` | Slash Commands |
| Event subscriptions | `/slack/events` | Event Subscriptions |
| Bot events | `app_mention`, `message.im` | Event Subscriptions > Bot Events |
| Bot scopes | `chat:write`, `commands`, `users:read`, `users:read.email` | OAuth & Permissions > Scopes |

## Bot Usage

Once installed, users can interact with the bot in two ways:

### Slash Commands

| Command | Description |
|---------|-------------|
| `/clawbake create` | Provision a new openclaw instance |
| `/clawbake status` | Show instance status, namespace, and URL |
| `/clawbake delete` | Delete your instance |
| `/clawbake help` | Show available commands |

### Messaging

- **@mention the bot** in any channel it's been added to
- **Send a direct message** to the bot

Messages are forwarded to the user's running openclaw instance. The user must have a running instance (via `/clawbake create`) before messaging will work.

## Troubleshooting

**Bot doesn't respond to events:**
- Verify the Request URL in Event Subscriptions shows a green checkmark
- Check that `SLACK_BOT_TOKEN` and `SLACK_SIGNING_SECRET` are both set (the bot won't start without both)
- Check server logs for signature verification failures

**"You don't have an openclaw instance" when messaging:**
- Run `/clawbake create` first, then wait for the instance to reach Running status

**Slash command returns an error:**
- Verify the Request URL in the slash command config points to `/slack/commands` (not `/slack/events`)
- Ensure the bot has the `commands` scope

**User email not found:**
- The bot maps Slack users to clawbake accounts by email. Ensure the Slack user has an email set in their profile and the bot has `users:read.email` scope.

**"request too old" errors:**
- The bot rejects requests with timestamps older than 5 minutes. Ensure the server's clock is accurate.

**Updating URLs after creation:**
- If your domain changes (e.g. new ngrok URL), update the URLs in two places in the Slack app settings: **Event Subscriptions** > Request URL, and **Slash Commands** > edit the `/clawbake` command's Request URL.
