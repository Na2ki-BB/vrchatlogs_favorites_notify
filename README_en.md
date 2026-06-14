# vrchatlogs-favorites-notify

A Raspberry Pi based physical notification device for VRChat friend events.

This tool listens to VRChat WebSocket events such as `friend-online`, `friend-offline`, and `friend-location`,
filters notification targets by a VRChat favorite friend group, and sends notifications to:

- Discord Webhook
- OLED display
- Local notification sound

It is designed to run on a Raspberry Pi with a small SSD1306 I2C OLED display, a speaker or earphones,
and physical switches for display/sound control.

> **Important (Consent & Privacy)**
> - This tool is intended to be used **only with the explicit consent** of the person being monitored/notified about.
> - Treat any received information such as userId, displayName, location, world name, and online state as sensitive.
> - Do not share collected or received information with third parties.
> - Never commit or publish VRChat auth cookies, twoFactorAuth cookies, or Discord webhook URLs.

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
- Displays latest notification on an SSD1306 OLED
- Supports Japanese text on OLED via Noto CJK font
- Plays a local notification sound
- Physical switch support:
  - GPIO17: display ON/OFF
  - GPIO27: sound ON/OFF
- When display is turned OFF, OLED is cleared
- When display is turned ON again, the latest notification is restored
- When sound is turned ON again, a test sound is played
- Automatically reconnects to VRChat WebSocket on disconnect

## Hardware

Tested target hardware:

- Raspberry Pi 3B
- 0.96 inch OLED
  - 128x64
  - I2C
  - SSD1306
  - Address: `0x3c`
- Speaker or earphones
- Physical switches
  - Display switch: GPIO17
  - Sound switch: GPIO27

GPIO wiring example:

```text
OLED VCC  -> Raspberry Pi 3.3V
OLED GND  -> Raspberry Pi GND
OLED SDA  -> GPIO2 / SDA / physical pin 3
OLED SCL  -> GPIO3 / SCL / physical pin 5

Display switch:
  one side -> GPIO17 / physical pin 11
  one side -> GND

Sound switch:
  one side -> GPIO27 / physical pin 13
  one side -> GND
```

The switches use Raspberry Pi internal pull-up resistors, so they should be connected between GPIO and GND.

## Requirements

### Go

* Go 1.20+ recommended

Go dependencies include:

* `github.com/gorilla/websocket`
* `periph.io/x/conn/v3`
* `periph.io/x/host/v3`

Install dependencies:

```bash
go mod tidy
```

### Python / OLED daemon

Required packages:

```bash
sudo apt update
sudo apt install -y python3-pip python3-pil i2c-tools fonts-noto-cjk fontconfig
python3 -m pip install --break-system-packages luma.oled
```

If your environment does not require `--break-system-packages`, this may also work:

```bash
python3 -m pip install luma.oled
```

Enable I2C:

```bash
sudo raspi-config
```

Then enable:

```text
Interface Options -> I2C
```

Check OLED address:

```bash
sudo i2cdetect -y 1
```

Expected result includes:

```text
3c
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

## Runtime Files

The Go process writes OLED display content to:

```text
runtime/oled_status.txt
```

The Python OLED daemon watches this file and updates the OLED display when it changes.

Recommended repository layout:

```text
vrchatlogs_favorites_notify/
├─ main.go
├─ oled_daemon.py
├─ runtime/
│  └─ .gitkeep
├─ env.example
├─ README.md
└─ .gitignore
```

Do not commit runtime output files.

Recommended `.gitignore`:

```gitignore
.env
*.env
runtime/*.txt
vrchatlogs_favorites_notify
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

## OLED Daemon

The OLED display is handled by a Python daemon.

Example manual run:

```bash
cd ~/vrchatlogs_favorites_notify
python3 oled_daemon.py
```

Manual display test:

```bash
printf "ONLINE\nTestFriend\n15:04\n" > runtime/oled_status.txt
```

If the OLED does not display anything, check:

```bash
sudo i2cdetect -y 1
```

and confirm that `0x3c` is visible.

## systemd Setup

This project uses two services:

1. OLED daemon service
2. VRChat notification watcher service

### OLED daemon service

Create:

```bash
sudo nano /etc/systemd/system/vrchat-oled.service
```

Example:

```ini
[Unit]
Description=VRChat OLED display daemon
After=multi-user.target

[Service]
Type=simple
User=satomi
WorkingDirectory=/home/satomi/vrchatlogs_favorites_notify
ExecStart=/usr/bin/python3 /home/satomi/vrchatlogs_favorites_notify/oled_daemon.py
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

If your Python file name is different from `oled_daemon.py`, update `ExecStart`.

### Go watcher service

Create:

```bash
sudo nano /etc/systemd/system/vrchatlogs-favorites-notify.service
```

Example:

```ini
[Unit]
Description=VRChat favorites notify watcher
After=network-online.target vrchat-oled.service
Wants=network-online.target vrchat-oled.service

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

Apply systemd changes:

```bash
sudo systemctl daemon-reload
```

Start services:

```bash
sudo systemctl start vrchat-oled.service
sudo systemctl start vrchatlogs-favorites-notify.service
```

Check status:

```bash
sudo systemctl status vrchat-oled.service --no-pager -l
sudo systemctl status vrchatlogs-favorites-notify.service --no-pager -l
```

Enable auto-start:

```bash
sudo systemctl enable vrchat-oled.service
sudo systemctl enable vrchatlogs-favorites-notify.service
```

View logs:

```bash
journalctl -u vrchat-oled.service -f
journalctl -u vrchatlogs-favorites-notify.service -f
```

## Operation

### Display switch

GPIO17 controls OLED display output.

```text
ON  -> OLED display enabled
OFF -> OLED is cleared
```

When the display is turned ON again, the latest notification is restored.

### Sound switch

GPIO27 controls notification sound.

```text
ON  -> sound enabled
OFF -> sound disabled
```

When the sound switch is turned ON again, a test sound is played.

### Discord

Discord notifications are sent regardless of display/sound switch state.

This is intentional so that physical output can be muted without stopping notification monitoring.

## Troubleshooting

### OLED does not display anything

Check I2C:

```bash
sudo i2cdetect -y 1
```

Expected:

```text
3c
```

Restart OLED daemon:

```bash
sudo systemctl restart vrchat-oled.service
sudo systemctl status vrchat-oled.service --no-pager -l
```

Check logs:

```bash
journalctl -u vrchat-oled.service -n 80 --no-pager
```

If the log says:

```text
can't open file '.../oled_daemon.py'
```

then the Python file name or path in `ExecStart` is wrong.

### Environment variables are not loaded

Check the Go service file includes:

```ini
EnvironmentFile=/etc/vrchat-logger.env
```

Check the file exists:

```bash
sudo cat /etc/vrchat-logger.env
```

Check service logs:

```bash
journalctl -u vrchatlogs-favorites-notify.service -n 80 --no-pager
```

If you see:

```text
VRC_AUTH_TOKEN not found
```

or:

```text
VRC_TWO_FACTOR_AUTH_TOKEN not found
```

then the environment file is missing, unreadable, or incorrectly configured.

### Sound does not play

Check the sound file:

```bash
ls -l /home/satomi/notify.wav
aplay /home/satomi/notify.wav
```

### Favorite list cannot be fetched

If the log includes:

```text
Requires Two-Factor Authentication
```

then `VRC_TWO_FACTOR_AUTH_TOKEN` is missing, expired, or incorrect.

Get a fresh `twoFactorAuth` cookie from a logged-in browser session and update:

```text
/etc/vrchat-logger.env
```

Then restart:

```bash
sudo systemctl restart vrchatlogs-favorites-notify.service
```

## Security Notes

* Never commit secrets such as:

  * `VRC_AUTH_TOKEN`
  * `VRC_TWO_FACTOR_AUTH_TOKEN`
  * `DISCORD_WEBHOOK_URL_FAVORITES`
* The WebSocket connection uses a URL that includes the auth token:
  `wss://pipeline.vrchat.cloud/?authToken=...`
* Be careful not to leak tokens through logs, screenshots, shell history, monitoring tools, or crash reports.
* If any token or webhook URL is exposed, rotate it immediately.
* `/etc/vrchat-logger.env` should be readable only by root.

## Disclaimer

Use at your own risk. Make sure your use complies with VRChat’s rules, Discord’s rules, and any other relevant service terms.
