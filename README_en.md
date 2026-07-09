# vrchatlogs-favorites-notify

A Raspberry Pi based notification device for VRChat friend events.

This tool listens to VRChat WebSocket events such as `friend-online`, `friend-offline`, and `friend-location`,
filters notification targets by a VRChat favorite friend group, and sends notifications to:

- Discord Webhook
- Local notification sound (speaker / earphones)
- OLED display (**optional / legacy**)

Notification sound ON/OFF is controlled via a small **HTTP API** exposed by the Go process.
Flip it from an **ESP32-C3 SuperMini + tact switch** kept at hand, or from a phone / `curl`.

> **Important (Consent & Privacy)**
> - This tool is intended to be used **only with the explicit consent** of the person being monitored/notified about.
> - Treat any received information such as userId, displayName, location, world name, and online state as sensitive.
> - Do not share collected or received information with third parties.
> - Never commit or publish VRChat auth cookies, twoFactorAuth cookies, or Discord webhook URLs.

> **Hardware change note (2026-07)**
> Earlier versions supported an OLED display and GPIO physical switches (GPIO17 = display / GPIO27 = sound).
> The OLED has been retired and the GPIO switches removed. Sound ON/OFF is now an HTTP API + ESP32 remote.
> The OLED / Python daemon sections are kept as "optional / legacy".

## Features

- Listens to VRChat WebSocket events:
  - `friend-online`
  - `friend-offline`
  - `friend-location`
- Uses a VRChat favorite friend group as the notification target list
- Refreshes favorite target list periodically
- Converts `userId` to `displayName`
- Converts `worldId` to world name when possible
- Sends Discord notifications
- Plays a local notification sound
- Controls sound ON/OFF via an **HTTP API** (`/sound/toggle`, etc.)
  - Operate it from an ESP32-C3 tact switch, a phone web page, `curl`, or any device.
  - State is persisted to `runtime/sound_enabled` and survives restarts.
- Displays latest notification on an SSD1306 OLED (**optional / legacy**)
- Automatically reconnects to VRChat WebSocket on disconnect

## Hardware

### Required

- Raspberry Pi (connected to your LAN)
- Speaker or earphones (played via `aplay`)

### Sound remote (recommended)

- ESP32-C3 SuperMini
- One tact switch

The ESP32 side is intentionally "thin" and holds no state. Pressing the switch just fires
a single HTTP request to the Raspberry Pi; the Pi owns the authoritative state.
To add another device/method later, just implement "send a request to the same URL".

ESP32-C3 SuperMini wiring example:

```text
tact switch : one side -> GPIO4 / other side -> GND   (internal pull-up, pressed = LOW)
onboard LED : GPIO8 (status; active LOW on most C3 SuperMini boards)
```

> GPIO8 / GPIO9 are strapping pins and GPIO20 / 21 are UART, so GPIO4 is used for the tact switch.
> The BOOT button (GPIO9) is left free for other features. Change `PIN_BUTTON` in `sound_switch.ino` to use a different pin.

### Optional / legacy (OLED)

- 0.96 inch OLED (128x64 / I2C / SSD1306 / address `0x3c`)

Wiring example if you use an OLED:

```text
OLED VCC  -> Raspberry Pi 3.3V
OLED GND  -> Raspberry Pi GND
OLED SDA  -> GPIO2 / SDA / physical pin 3
OLED SCL  -> GPIO3 / SCL / physical pin 5
```

## Requirements

### Go

* Go 1.20+ recommended

Go dependencies:

* `github.com/gorilla/websocket`

Install dependencies:

```bash
go mod tidy
```

### Arduino (ESP32-C3 remote)

* Arduino IDE (or arduino-cli)
* Add "esp32" (Espressif Systems) via Boards Manager
* Board: `ESP32C3 Dev Module`

Edit the top of `esp32/sound_switch/sound_switch.ino`:

```cpp
static const char *WIFI_SSID = "YOUR_WIFI_SSID";
static const char *WIFI_PASS = "YOUR_WIFI_PASSWORD";
static const char *PI_HOST   = "192.168.0.10";  // Raspberry Pi IP
static const int   PI_PORT   = 8099;            // SOUND_API_ADDR port
```

After flashing, pressing the tact switch toggles the notification sound. The onboard LED shows the current state.

### Python / OLED daemon (optional / legacy)

Only needed if you use an OLED.

```bash
sudo apt update
sudo apt install -y python3-pip python3-pil i2c-tools fonts-noto-cjk fontconfig
python3 -m pip install --break-system-packages luma.oled
```

Enable I2C (`sudo raspi-config` -> `Interface Options` -> `I2C`), then check the address:

```bash
sudo i2cdetect -y 1   # 3c should be visible
```

## Environment Variables

This project uses an environment file located at:

```text
/etc/vrchat-logger.env
```

Example:

```bash
VRC_AUTH_TOKEN=YOUR_VRCHAT_AUTH_COOKIE
VRC_TWO_FACTOR_AUTH_TOKEN=YOUR_VRCHAT_TWO_FACTOR_AUTH_COOKIE
VRC_NOTIFY_FAVORITE_TAG=group_0
DISCORD_WEBHOOK_URL_FAVORITES=https://discord.com/api/webhooks/...
SOUND_API_ADDR=:8099
```

### Variables

Required:

* `VRC_AUTH_TOKEN`

  * VRChat `auth` cookie value
  * Used for WebSocket and API access

* `VRC_TWO_FACTOR_AUTH_TOKEN`

  * VRChat `twoFactorAuth` cookie value
  * Required for fetching favorite friend list

* `VRC_NOTIFY_FAVORITE_TAG`

  * VRChat favorite friend group tag used as notification target
  * Example: `group_0`

Optional:

* `DISCORD_WEBHOOK_URL_FAVORITES`

  * Discord webhook URL
  * If empty, Discord notifications are disabled

* `SOUND_API_ADDR`

  * Listen address for the sound ON/OFF control API
  * Default: `:8099` (all interfaces)
  * Example: `0.0.0.0:8099`, `192.168.0.10:8099`
  * Make sure it is reachable from the ESP32 / phone over the LAN (allow it in your firewall).

* `HUB_DATA_DIR`

  * Directory path to write hub JSON data for [raspi-esp32-status-panel](https://github.com/na2ki/raspi-esp32-status-panel) integration
  * If empty, hub integration is disabled and behavior is unchanged
  * Example: `/home/na2ki/raspi-esp32-status-panel/hub/data`

Create environment file:

```bash
sudo nano /etc/vrchat-logger.env
```

Recommended permissions:

```bash
sudo chown root:root /etc/vrchat-logger.env
sudo chmod 600 /etc/vrchat-logger.env
```

## Sound Control API

The Go process holds the sound ON/OFF state and exposes an HTTP API on the LAN.
State-changing endpoints (`toggle` / `on` / `off`) accept **both GET and POST**, so any device can operate it with a single request.

| Method | Path | Action |
| --- | --- | --- |
| GET | `/sound` | Return current state (`{"enabled":true}`) |
| GET/POST | `/sound/toggle` | Flip ON/OFF |
| GET/POST | `/sound/on` | Turn ON |
| GET/POST | `/sound/off` | Turn OFF |
| GET | `/` | Minimal web page you can tap from a phone |

Examples (`<pi>` is the Raspberry Pi IP, port from `SOUND_API_ADDR`):

```bash
curl http://<pi>:8099/sound              # check state
curl -X POST http://<pi>:8099/sound/toggle
curl -X POST http://<pi>:8099/sound/off
```

- State is persisted to `runtime/sound_enabled` (`1` / `0`) and survives restarts.
- If the state file is missing, the default is **ON** (so a network outage never leaves you permanently muted).
- When switching from OFF to ON, a single confirmation test sound is played.

## Runtime Files

The following files are generated under `runtime/` at runtime (none need to be committed):

```text
runtime/sound_enabled     persisted sound ON/OFF state (1 / 0)
runtime/oled_status.txt   OLED display content (only if you use an OLED / legacy)
```

Recommended repository layout:

```text
vrchatlogs_favorites_notify/
├─ main.go
├─ oled_notify.py                 # optional / legacy
├─ esp32/
│  └─ sound_switch/
│     └─ sound_switch.ino
├─ runtime/
│  └─ .gitkeep
├─ .env.example
├─ README.md
└─ .gitignore
```

Recommended `.gitignore`:

```gitignore
.env
*.env
runtime/*
!runtime/.gitkeep
```

## Notification Sound

The Go program plays:

```text
/home/satomi/notify.wav
```

Example setup:

```bash
find /usr/share/sounds -name "*.wav" | head
cp /usr/share/sounds/alsa/Front_Center.wav /home/satomi/notify.wav
aplay /home/satomi/notify.wav
```

You can replace `/home/satomi/notify.wav` with any short `.wav` notification sound.

## Build

```bash
cd ~/vrchatlogs_favorites_notify
go build -o vrchatlogs_favorites_notify
```

Run manually:

```bash
./vrchatlogs_favorites_notify
```

## OLED Daemon (optional / legacy)

Only used if you have an OLED connected. The Go process writes OLED content to `runtime/oled_status.txt`,
and the Python daemon watches that file and updates the OLED.

Example manual run:

```bash
cd ~/vrchatlogs_favorites_notify
python3 oled_notify.py
```

Manual display test:

```bash
printf "ONLINE\nTestFriend\n15:04\n" > runtime/oled_status.txt
```

## systemd Setup

Set up the VRChat notification watcher service. Add the OLED daemon service only if you use an OLED.

### Go watcher service

Create:

```bash
sudo nano /etc/systemd/system/vrchatlogs-favorites-notify.service
```

Example:

```ini
[Unit]
Description=VRChat favorites notify watcher
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=satomi
WorkingDirectory=/home/satomi/vrchatlogs_favorites_notify
EnvironmentFile=/etc/vrchat-logger.env
ExecStart=/home/satomi/vrchatlogs_favorites_notify/vrchatlogs_favorites_notify
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### OLED daemon service (optional / legacy)

Create only if you use an OLED.

```bash
sudo nano /etc/systemd/system/vrchat-oled.service
```

```ini
[Unit]
Description=VRChat OLED display daemon
After=multi-user.target

[Service]
Type=simple
User=satomi
WorkingDirectory=/home/satomi/vrchatlogs_favorites_notify
ExecStart=/usr/bin/python3 /home/satomi/vrchatlogs_favorites_notify/oled_notify.py
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

Apply, start, enable:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now vrchatlogs-favorites-notify.service
# only if you use an OLED:
# sudo systemctl enable --now vrchat-oled.service
```

Status / logs:

```bash
sudo systemctl status vrchatlogs-favorites-notify.service --no-pager -l
journalctl -u vrchatlogs-favorites-notify.service -f
```

## Operation

### Sound ON/OFF

- Press the ESP32-C3 tact switch -> toggle (LED shows current state)
- Open `http://<pi>:8099/` in a phone / PC browser -> ON / OFF / TOGGLE
- From any device: `curl -X POST http://<pi>:8099/sound/toggle`

### Discord

Discord notifications are sent regardless of the sound ON/OFF state.

This is intentional so that local sound output can be muted without stopping notification monitoring.

## Troubleshooting

### Sound does not play

First check the sound file and ALSA:

```bash
ls -l /home/satomi/notify.wav
aplay /home/satomi/notify.wav
```

Then check the sound state:

```bash
curl http://<pi>:8099/sound     # is it {"enabled":true}?
cat runtime/sound_enabled       # 1 means ON
```

If it is `{"enabled":false}`, it is OFF. Turn it back on:

```bash
curl -X POST http://<pi>:8099/sound/on
```

### ESP32 cannot toggle

- Check Wi-Fi connection and HTTP response code on the ESP32 serial monitor (115200).
- Confirm `PI_HOST` / `PI_PORT` match the Raspberry Pi IP and `SOUND_API_ADDR`.
- Confirm the API is listening on the Raspberry Pi:

```bash
curl http://<pi>:8099/sound
sudo ss -ltnp | grep 8099
```

### Environment variables are not loaded

Check the Go service file includes:

```ini
EnvironmentFile=/etc/vrchat-logger.env
```

Check service logs:

```bash
journalctl -u vrchatlogs-favorites-notify.service -n 80 --no-pager
```

If you see the following, the environment file is missing, unreadable, or misconfigured:

```text
VRC_AUTH_TOKEN not found
VRC_TWO_FACTOR_AUTH_TOKEN not found
```

### Favorite list cannot be fetched

If the log includes:

```text
Requires Two-Factor Authentication
```

then `VRC_TWO_FACTOR_AUTH_TOKEN` is missing, expired, or incorrect.
Get a fresh `twoFactorAuth` cookie from a logged-in browser session, update `/etc/vrchat-logger.env`, and restart:

```bash
sudo systemctl restart vrchatlogs-favorites-notify.service
```

### OLED does not display anything (optional / legacy)

```bash
sudo i2cdetect -y 1                         # is 3c visible?
sudo systemctl restart vrchat-oled.service
journalctl -u vrchat-oled.service -n 80 --no-pager
```

## Security Notes

* Never commit secrets such as:

  * `VRC_AUTH_TOKEN`
  * `VRC_TWO_FACTOR_AUTH_TOKEN`
  * `DISCORD_WEBHOOK_URL_FAVORITES`
* The WebSocket connection uses a URL that includes the auth token:
  `wss://pipeline.vrchat.cloud/?authToken=...`
* The sound control API (`SOUND_API_ADDR`) has no authentication. Expose it **only on a trusted LAN** and never to the internet.
* Be careful not to leak tokens through logs, screenshots, shell history, monitoring tools, or crash reports.
* If any token or webhook URL is exposed, rotate it immediately.
* `/etc/vrchat-logger.env` should be readable only by root.

## Disclaimer

Use at your own risk. Make sure your use complies with VRChat’s rules, Discord’s rules, and any other relevant service terms.
