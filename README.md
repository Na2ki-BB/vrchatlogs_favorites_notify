
# vrchat-friend-notify (WebSocket → Discord)

A small tool that listens to VRChat WebSocket events (`friend-online`, `friend-offline`, `friend-location`)
and sends notifications to a Discord webhook when a specific user’s state changes.

> **Important (Consent & Privacy)**
> - This tool is intended to be used **only with the explicit consent** of the person being monitored/notified about.
> - Treat any received information (userId / location / world, etc.) as sensitive and do not share it with third parties.

## Features
- Monitors `friend-online` / `friend-offline` / `friend-location` events
- Sends Discord notifications **only** when `userId` matches `VRC_TARGET_USERID`
- Automatically reconnects on disconnect

## Requirements
- Go 1.20+ (recommended)
- Discord Webhook (optional)

## Setup

### 1) Configure environment variables
This repository includes `.env.example` (dummy values).

Required:
- `VRC_TARGET_USERID` : Target VRChat `userId` to watch
- `VRC_AUTH_TOKEN` : VRChat `authToken` (**never commit or publish this**)

Optional:
- `DISCORD_WEBHOOK_URL` : Discord webhook URL (if empty, Discord notifications are disabled)

Example (bash):
```bash
export VRC_TARGET_USERID="usr_xxxxxxxxxxxxxxxxx"
export VRC_AUTH_TOKEN="YOUR_TOKEN"
export DISCORD_WEBHOOK_URL="https://discord.com/api/webhooks/...."
````

### 2) Run

```bash
go run .
```

Build:

```bash
go build -o vrchat-notify .
./vrchat-notify
```

## Security Notes

* Never commit secrets like `VRC_AUTH_TOKEN` or `DISCORD_WEBHOOK_URL`.
* This tool connects using a URL that includes the token as a query parameter:
  `wss://pipeline.vrchat.cloud/?authToken=...`
  Be careful not to leak URLs via logs, monitoring, proxies, crash reports, etc.
* If you suspect the token was exposed, rotate/regenerate it immediately.

## Disclaimer

* Use at your own risk. Make sure your use complies with the relevant services’ terms and rules.

