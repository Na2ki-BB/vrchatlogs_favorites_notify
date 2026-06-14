# vrchatlogs-favorites-notify

VRChatのフレンドイベントを通知する、Raspberry Piベースの物理通知端末です。

このツールは、VRChat WebSocketの `friend-online`、`friend-offline`、`friend-location` などのイベントを監視し、
VRChatのお気に入りフレンドグループをもとに通知対象を絞り込み、以下へ通知します。

- Discord Webhook
- OLEDディスプレイ
- ローカル通知音

小型のSSD1306 I2C OLEDディスプレイ、スピーカーまたはイヤホン、
および画面/音声制御用の物理スイッチを接続したRaspberry Pi上で動作することを想定しています。

> **重要（同意とプライバシー）**
> - このツールは、監視・通知対象となる相手の**明示的な同意がある場合にのみ**使用することを想定しています。
> - 受信した userId、displayName、location、ワールド名、オンライン状態などの情報は機密性のある情報として扱ってください。
> - 収集・受信した情報を第三者に共有しないでください。
> - VRChatの auth cookie、twoFactorAuth cookie、Discord Webhook URL を絶対にコミット・公開しないでください。

## Features

- VRChat WebSocketイベントを監視します。
  - `friend-online`
  - `friend-offline`
  - `friend-location`
- VRChatのお気に入りフレンドグループを通知対象リストとして使用します。
- 通知対象リストを定期的に再取得します。
- `userId` を `displayName` に変換します。
- 可能な場合、`worldId` をワールド名に変換します。
- Discord通知を送信します。
- 最新の通知をSSD1306 OLEDに表示します。
- Noto CJKフォントを使用して、OLED上の日本語表示に対応しています。
- ローカル通知音を再生します。
- 物理スイッチに対応しています。
  - GPIO17: 画面 ON/OFF
  - GPIO27: 音声 ON/OFF
- 画面をOFFにすると、OLEDを消灯します。
- 画面をONに戻すと、最新の通知内容を復元します。
- 音声をONに戻すと、テスト音を再生します。
- VRChat WebSocket切断時に自動で再接続します。

## Hardware

動作確認対象のハードウェア:

- Raspberry Pi 3B
- 0.96インチ OLED
  - 128x64
  - I2C
  - SSD1306
  - アドレス: `0x3c`
- スピーカーまたはイヤホン
- 物理スイッチ
  - 画面スイッチ: GPIO17
  - 音声スイッチ: GPIO27

GPIO配線例:

```text
OLED VCC  -> Raspberry Pi 3.3V
OLED GND  -> Raspberry Pi GND
OLED SDA  -> GPIO2 / SDA / 物理ピン 3
OLED SCL  -> GPIO3 / SCL / 物理ピン 5

画面スイッチ:
  片方 -> GPIO17 / 物理ピン 11
  片方 -> GND

音声スイッチ:
  片方 -> GPIO27 / 物理ピン 13
  片方 -> GND
````

スイッチはRaspberry Piの内部プルアップ抵抗を使用するため、GPIOとGNDの間に接続します。

## Requirements

### Go

* Go 1.20+ 推奨

Goの依存関係:

* `github.com/gorilla/websocket`
* `periph.io/x/conn/v3`
* `periph.io/x/host/v3`

依存関係のインストール:

```bash
go mod tidy
```

### Python / OLED daemon

必要なパッケージ:

```bash
sudo apt update
sudo apt install -y python3-pip python3-pil i2c-tools fonts-noto-cjk fontconfig
python3 -m pip install --break-system-packages luma.oled
```

環境によっては、`--break-system-packages` なしでも動作します。

```bash
python3 -m pip install luma.oled
```

I2Cを有効化します。

```bash
sudo raspi-config
```

以下を有効化します。

```text
Interface Options -> I2C
```

OLEDのアドレスを確認します。

```bash
sudo i2cdetect -y 1
```

期待される結果には以下が含まれます。

```text
3c
```

## Environment Variables

このプロジェクトでは、以下の環境変数ファイルを使用します。

```text
/etc/vrchat-logger.env
```

例:

```bash
VRC_AUTH_TOKEN=YOUR_VRCHAT_AUTH_COOKIE
VRC_TWO_FACTOR_AUTH_TOKEN=YOUR_VRCHAT_TWO_FACTOR_AUTH_COOKIE
VRC_NOTIFY_FAVORITE_TAG=group_0
DISCORD_WEBHOOK_URL_FAVORITES=https://discord.com/api/webhooks/...
```

### Variables

必須:

* `VRC_AUTH_TOKEN`

  * VRChatの `auth` cookie の値
  * WebSocket接続およびAPIアクセスに使用します。

* `VRC_TWO_FACTOR_AUTH_TOKEN`

  * VRChatの `twoFactorAuth` cookie の値
  * お気に入りフレンドリストの取得に必要です。

* `VRC_NOTIFY_FAVORITE_TAG`

  * 通知対象として使用するVRChatのお気に入りフレンドグループのタグ
  * 例: `group_0`

任意:

* `DISCORD_WEBHOOK_URL_FAVORITES`

  * Discord Webhook URL
  * 空の場合、Discord通知は無効になります。

* `HUB_DATA_DIR`

  * [raspi-esp32-status-panel](https://github.com/na2ki/raspi-esp32-status-panel) との連携用のJSONデータを書き込むディレクトリパス
  * 空の場合、ハブ連携は無効になり、動作はそのままです
  * 例: `/home/na2ki/raspi-esp32-status-panel/hub/data`

環境変数ファイルを作成します。

```bash
sudo nano /etc/vrchat-logger.env
```

推奨権限:

```bash
sudo chown root:root /etc/vrchat-logger.env
sudo chmod 600 /etc/vrchat-logger.env
```

## Runtime Files

Goプロセスは、OLED表示内容を以下のファイルに書き込みます。

```text
runtime/oled_status.txt
```

Python OLED daemonはこのファイルを監視し、内容が変更されたらOLEDディスプレイを更新します。

推奨リポジトリ構成:

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

実行時に生成されるファイルはコミットしないでください。

推奨 `.gitignore`:

```gitignore
.env
*.env
runtime/*.txt
vrchatlogs_favorites_notify
```

## Notification Sound

Goプログラムは以下のファイルを再生します。

```text
/home/satomi/notify.wav
```

設定例:

```bash
find /usr/share/sounds -name "*.wav" | head
cp /usr/share/sounds/alsa/Front_Center.wav /home/satomi/notify.wav
aplay /home/satomi/notify.wav
```

`/home/satomi/notify.wav` は、任意の短い `.wav` 通知音に置き換えることができます。

## Build

```bash
cd ~/vrchatlogs_favorites_notify
go build -o vrchatlogs_favorites_notify
```

手動実行:

```bash
./vrchatlogs_favorites_notify
```

## OLED Daemon

OLED表示はPython daemonが担当します。

手動実行例:

```bash
cd ~/vrchatlogs_favorites_notify
python3 oled_daemon.py
```

手動表示テスト:

```bash
printf "ONLINE\nTestFriend\n15:04\n" > runtime/oled_status.txt
```

OLEDに何も表示されない場合は、以下を確認します。

```bash
sudo i2cdetect -y 1
```

`0x3c` が表示されていることを確認してください。

## systemd Setup

このプロジェクトでは2つのサービスを使用します。

1. OLED daemon service
2. VRChat notification watcher service

### OLED daemon service

作成:

```bash
sudo nano /etc/systemd/system/vrchat-oled.service
```

例:

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

Pythonファイル名が `oled_daemon.py` と異なる場合は、`ExecStart` を更新してください。

### Go watcher service

作成:

```bash
sudo nano /etc/systemd/system/vrchatlogs-favorites-notify.service
```

例:

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

systemdの変更を反映します。

```bash
sudo systemctl daemon-reload
```

サービスを起動します。

```bash
sudo systemctl start vrchat-oled.service
sudo systemctl start vrchatlogs-favorites-notify.service
```

ステータスを確認します。

```bash
sudo systemctl status vrchat-oled.service --no-pager -l
sudo systemctl status vrchatlogs-favorites-notify.service --no-pager -l
```

自動起動を有効化します。

```bash
sudo systemctl enable vrchat-oled.service
sudo systemctl enable vrchatlogs-favorites-notify.service
```

ログを表示します。

```bash
journalctl -u vrchat-oled.service -f
journalctl -u vrchatlogs-favorites-notify.service -f
```

## Operation

### Display switch

GPIO17はOLED表示出力を制御します。

```text
ON  -> OLED表示を有効化
OFF -> OLEDを消灯
```

画面をONに戻すと、最新の通知内容が復元されます。

### Sound switch

GPIO27は通知音を制御します。

```text
ON  -> 音声有効
OFF -> 音声無効
```

音声スイッチをONに戻すと、テスト音が再生されます。

### Discord

Discord通知は、画面/音声スイッチの状態に関係なく送信されます。

これは、通知監視自体を止めずに、物理出力だけをミュートできるようにするための意図的な仕様です。

## Troubleshooting

### OLEDに何も表示されない

I2Cを確認します。

```bash
sudo i2cdetect -y 1
```

期待される結果:

```text
3c
```

OLED daemonを再起動します。

```bash
sudo systemctl restart vrchat-oled.service
sudo systemctl status vrchat-oled.service --no-pager -l
```

ログを確認します。

```bash
journalctl -u vrchat-oled.service -n 80 --no-pager
```

ログに以下のような表示がある場合:

```text
can't open file '.../oled_daemon.py'
```

Pythonファイル名または `ExecStart` のパスが間違っています。

### 環境変数が読み込まれない

Goサービスファイルに以下が含まれていることを確認します。

```ini
EnvironmentFile=/etc/vrchat-logger.env
```

ファイルが存在することを確認します。

```bash
sudo cat /etc/vrchat-logger.env
```

サービスログを確認します。

```bash
journalctl -u vrchatlogs-favorites-notify.service -n 80 --no-pager
```

以下のような表示がある場合:

```text
VRC_AUTH_TOKEN not found
```

または:

```text
VRC_TWO_FACTOR_AUTH_TOKEN not found
```

環境変数ファイルが存在しない、読み取れない、または設定が間違っています。

### 音が鳴らない

音声ファイルを確認します。

```bash
ls -l /home/satomi/notify.wav
aplay /home/satomi/notify.wav
```

### お気に入りリストを取得できない

ログに以下が含まれる場合:

```text
Requires Two-Factor Authentication
```

`VRC_TWO_FACTOR_AUTH_TOKEN` が存在しない、期限切れ、または間違っています。

ログイン済みのブラウザセッションから新しい `twoFactorAuth` cookie を取得し、以下を更新してください。

```text
/etc/vrchat-logger.env
```

その後、サービスを再起動します。

```bash
sudo systemctl restart vrchatlogs-favorites-notify.service
```

## Security Notes

* 以下のような機密情報は絶対にコミットしないでください。

  * `VRC_AUTH_TOKEN`
  * `VRC_TWO_FACTOR_AUTH_TOKEN`
  * `DISCORD_WEBHOOK_URL_FAVORITES`
* WebSocket接続では、auth tokenを含むURLを使用します。
  `wss://pipeline.vrchat.cloud/?authToken=...`
* ログ、スクリーンショット、シェル履歴、監視ツール、クラッシュレポートなどからトークンが漏れないよう注意してください。
* トークンまたはWebhook URLが漏えいした疑いがある場合は、ただちにローテーションしてください。
* `/etc/vrchat-logger.env` はrootのみが読み取れるようにしてください。

## Disclaimer

自己責任で使用してください。VRChat、Discord、および関連するサービスのルールや利用規約に従って使用してください。

