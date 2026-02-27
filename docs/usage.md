# Usage Guide

This guide covers how to use Clawbake after it's been deployed. For deployment instructions, see [deployment.md](deployment.md).

## Roles

Clawbake has two roles:

- **Admin**: Can view all instances, manage users, and configure instance defaults.
- **User**: Can create and manage their own instance.

The first user to log in is automatically assigned the admin role. Subsequent users are assigned the user role. Admins can promote users via the database.

## Logging In

If OIDC is configured, users log in with their Google account. If OIDC is not configured (local development), a simple login screen lets you choose a role.

## Dashboard

After logging in, the dashboard shows your instances (or all instances for admins). Each instance card displays:

- Current status (Pending, Creating, Starting, Running, Failed, Terminating)
- Owner name (admin view only)
- Container image
- Links to the Web UI and Terminal (when the instance is running)

Status badges auto-refresh every 5 seconds while the instance is starting up.

## Creating an Instance

Each user can have one instance at a time. Click **Create Instance** on the dashboard to open the creation dialog.

### Placeholders

If the admin has configured gateway config placeholders (see [Admin: Instance Defaults](#instance-defaults)), the creation dialog will show required input fields for each placeholder. All placeholder fields must be filled before creating.

For example, if the admin gateway config template contains `{{API_KEY}}`, you'll see a required "API_KEY" input field.

### Gateway Config Override

The creation dialog also includes an optional **Gateway Config Override** textarea. This accepts a JSON object that is deep-merged over the admin defaults (after placeholder substitution). Use this to customize your instance without changing the shared defaults.

For example, to add a provider:
```json
{
  "gateway": {
    "providers": {
      "openai": {
        "apiKey": "sk-..."
      }
    }
  }
}
```

The merge is recursive -- nested objects are merged key-by-key, while scalar values and arrays are replaced entirely.

## Accessing Your Instance

Once an instance reaches the **Running** phase, two access methods are available:

### Web UI

Click **Web UI** on the dashboard or instance detail page. This opens the openclaw web interface through `/proxy/web/`. Authentication is handled automatically -- the proxy injects your gateway token.

### Terminal (ttyd)

If ttyd is enabled, click **Terminal** to open a browser-based terminal through `/proxy/tui/`. This runs the openclaw TUI via ttyd and is fully interactive.

## Instance Detail

Click **Details** on any instance card to see the full instance detail page, which shows:

- Status badge with live refresh
- Spec details: image, CPU/memory requests and limits, storage size, gateway token
- Kubernetes conditions and their status
- Access buttons (Web UI, Terminal) when running
- Delete button with confirmation

## Deleting an Instance

Click **Delete** on the instance card or detail page. A confirmation dialog will appear. Deletion removes the Kubernetes namespace and all associated resources (deployment, PVC, service, ingress).

## Slack Bot

If the Slack integration is enabled (see [slack-setup.md](slack-setup.md)), users can interact with Clawbake via slash commands and direct messages.

### Slash Commands

| Command | Description |
|---------|-------------|
| `/clawbake create` | Create a new instance |
| `/clawbake create KEY=value` | Create with placeholder values |
| `/clawbake create json={"key":"value"}` | Create with gateway config override |
| `/clawbake create KEY=value json={...}` | Create with both placeholders and override |
| `/clawbake status` | Show your instance status and namespace |
| `/clawbake open` | Get a link to your instance web UI |
| `/clawbake open tui` | Get a link to your instance terminal |
| `/clawbake delete` | Delete your instance |
| `/clawbake help` | Show available commands |

The bot automatically adapts its help text and command references to whatever the slash command is named in your Slack app.

### Direct Messages

Send a direct message to the bot (or @mention it in a channel) and it will forward your message to your running openclaw instance as a chat completion request. The bot maps Slack users to Clawbake accounts by email address.

## Admin Features

Admin-only pages are accessible from the navigation bar.

### Users

The **Users** page (`/ui/admin/users`) lists all registered users with their name, email, and role.

### Instance Defaults

The **Defaults** page (`/ui/admin/defaults`) configures the default settings applied to every new instance:

| Setting | Description | Example |
|---------|-------------|---------|
| Image | Docker image for openclaw | `ghcr.io/openclaw/openclaw:latest` |
| CPU Request | Kubernetes CPU request | `500m` |
| Memory Request | Kubernetes memory request | `1Gi` |
| CPU Limit | Kubernetes CPU limit | `2000m` |
| Memory Limit | Kubernetes memory limit | `2Gi` |
| Storage Size | PVC size | `5Gi` |
| Gateway Config | JSON config for the openclaw gateway | See below |

### Gateway Config Placeholders

The gateway config field supports template placeholders using `{{PLACEHOLDER_NAME}}` syntax. Placeholder names must be uppercase letters, numbers, and underscores (starting with a letter).

When placeholders are present:
- The creation dialog shows a required input for each placeholder
- The Slack bot's `create` command requires `KEY=value` pairs for each
- The `help` command shows the required parameters

This lets admins define a config template that requires users to supply their own values (e.g., API keys) at creation time, without exposing the full config structure.

**Example template:**
```json
{
  "gateway": {
    "providers": {
      "openai": {
        "apiKey": "{{OPENAI_API_KEY}}"
      }
    }
  }
}
```

Users would then create with: `/clawbake create OPENAI_API_KEY=sk-...`

The **Format JSON** button pretty-prints the config, and **Reset to Default** restores the built-in default config. Validation checks that the template produces valid JSON when placeholders are substituted.

## API

Clawbake also exposes a REST API for programmatic access. All endpoints require authentication via session cookie.

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/instances` | List instances (filtered by role) |
| GET | `/api/instances/{id}` | Get instance details |
| POST | `/api/instances` | Create instance |
| DELETE | `/api/instances/{id}` | Delete instance |
| GET | `/api/admin/users` | List users (admin only) |
| GET | `/api/admin/defaults` | Get instance defaults (admin only) |
| POST | `/api/admin/defaults` | Update instance defaults (admin only) |
