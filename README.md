# vrchatlogs-favorites-notify

VRChatのフレンドイベントを通知する、Raspberry Piベースの通知端末です。

このツールは、VRChat WebSocketの `friend-online`、`friend-offline`、`friend-location` などのイベントを監視し、
VRChatのお気に入りフレンドグループをもとに通知対象を絞り込み、以下へ通知します。

- Discord Webhook
- ローカル通知音（スピーカー / イヤホン）
- OLEDディスプレイ（**任意 / レガシー**）

通知音の ON/OFF は、Goが公開する小さな **HTTP API** で制御します。
手元に置いた **ESP32-C3 SuperMini + タクトスイッチ** から、あるいはスマホ / `curl` から、そのAPIを叩いて切り替えます。

> **重要（同意とプライバシー）**
> - このツールは、監視・通知対象となる相手の**明示的な同意がある場合にのみ**使用することを想定しています。
> - 受信した userId、displayName、location、ワールド名、オンライン状態などの情報は機密性のある情報として扱ってください。
> - 収集・受信した情報を第三者に共有しないでください。
> - VRChatの auth cookie、twoFactorAuth cookie、Discord Webhook URL を絶対にコミット・公開しないでください。

> **ハードウェア構成の変更について（2026-07）**
> 以前の版は OLED表示 と GPIO物理スイッチ（GPIO17=画面 / GPIO27=音）に対応していました。
> 現在は OLED を撤去し、GPIOスイッチも廃止しています。通知音の ON/OFF は HTTP API + ESP32 リモートに置き換わりました。
> OLED / Python daemon の記述は「任意 / レガシー」として残しています。

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
- ローカル通知音を再生します。
- 通知音の ON/OFF を **HTTP API** で制御します（`/sound/toggle` など）。
  - ESP32-C3 のタクトスイッチ、スマホのWebページ、`curl` など、どの端末からでも操作できます。
  - 状態は `runtime/sound_enabled` に永続化され、再起動後も維持されます。
- 最新の通知内容を SSD1306 OLED に表示します（**任意 / レガシー**）。
- VRChat WebSocket切断時に自動で再接続します。

## Hardware

### 必須

- Raspberry Pi（有線/無線でLANに接続）
- スピーカーまたはイヤホン（`aplay` で再生）

### 音声リモート（推奨）

- ESP32-C3 SuperMini
- タクトスイッチ 1個

ESP32側は状態を持たない「薄い」実装です。スイッチを押すと Raspberry Pi の HTTP API に
リクエストを1回投げるだけで、状態の正はすべて Raspberry Pi 側が保持します。
将来、別の端末や方法でON/OFFしたくなっても「同じURLにリクエストを投げる」だけで拡張できます。

ESP32-C3 SuperMini 配線例:

```text
タクトスイッチ: 片方 -> GPIO4 / もう片方 -> GND   （内部プルアップ、押下=LOW）
オンボードLED : GPIO8（状態表示。多くのC3 SuperMiniでアクティブLOW）
```

> GPIO8 / GPIO9 はstrappingピン、GPIO20 / 21 はUARTのため、タクトスイッチには GPIO4 を使用します。
> BOOTボタン（GPIO9）は別機能に残せます。別ピンを使う場合は `sound_switch.ino` の `PIN_BUTTON` を変更してください。

### 任意 / レガシー（OLED）

- 0.96インチ OLED（128x64 / I2C / SSD1306 / アドレス `0x3c`）

OLEDを使う場合の配線例:

```text
OLED VCC  -> Raspberry Pi 3.3V
OLED GND  -> Raspberry Pi GND
OLED SDA  -> GPIO2 / SDA / 物理ピン 3
OLED SCL  -> GPIO3 / SCL / 物理ピン 5
```

## Requirements

### Go

* Go 1.20+ 推奨

Goの依存関係:

* `github.com/gorilla/websocket`

依存関係のインストール:

```bash
go mod tidy
```

### Arduino（ESP32-C3 リモート）

* Arduino IDE（または arduino-cli）
* ボードマネージャに「esp32」（Espressif Systems）を追加
* ボード選択: `ESP32C3 Dev Module`

Wi-Fi等の設定は `esp32/sound_switch/secrets.h`（**Git管理外**）に書きます。テンプレートをコピーして編集してください。

```bash
cd esp32/sound_switch
cp secrets.h.example secrets.h
```

```cpp
// secrets.h
#define WIFI_SSID "YOUR_WIFI_SSID"      // 2.4GHz帯のSSID（C3は5GHz非対応）
#define WIFI_PASS "YOUR_WIFI_PASSWORD"
#define PI_HOST   "192.168.0.10"        // Raspberry Pi のIP
#define PI_PORT   8099                  // SOUND_API_ADDR のポート
```

> `secrets.h` は `.gitignore` 済みなので、Wi‑Fiパスワードが誤ってコミット・公開されることはありません。`sound_switch.ino` はこの `secrets.h` を `#include` します。

書き込み後、タクトスイッチを押すと通知音がトグルします。オンボードLEDが現在の状態を表示します。

### Python / OLED daemon（任意 / レガシー）

OLEDを使う場合のみ必要です。

```bash
sudo apt update
sudo apt install -y python3-pip python3-pil i2c-tools fonts-noto-cjk fontconfig
python3 -m pip install --break-system-packages luma.oled
```

I2Cを有効化し（`sudo raspi-config` -> `Interface Options` -> `I2C`）、アドレスを確認します。

```bash
sudo i2cdetect -y 1   # 3c が見えればOK
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
VRC_USERNAME=YOUR_LOGIN_USERNAME
VRC_PASSWORD=YOUR_PASSWORD
VRC_TOTP_SECRET=YOUR_BASE32_TOTP_SEED
VRC_USER_AGENT="vrchat-friendnotify-auth/1.0 contact@example.com"
VRC_NOTIFY_FAVORITE_TAG=group_0
DISCORD_WEBHOOK_URL_FAVORITES=https://discord.com/api/webhooks/...
SOUND_API_ADDR=:8099
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

* `SOUND_API_ADDR`

  * 通知音 ON/OFF 制御APIの待ち受けアドレス
  * 既定値: `:8099`（全インターフェース）
  * 例: `0.0.0.0:8099`、`192.168.0.10:8099`
  * ESP32やスマホからLAN経由で届くこと（ファイアウォールで許可）を確認してください。

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

## Auth Cookie Generator

`cmd/vrchat-auth-cookies` は、VRChatのユーザー名・パスワードとTOTPシークレットからログインし、
通知サービスが必要とする `auth` / `twoFactorAuth` Cookieを生成します。

認証順序は次のとおりです。

1. Basic認証で `GET /auth/user` を呼び、`auth` Cookieを取得します。
2. 同じセッションでTOTPコードを1回生成し、`POST /auth/twofactorauth/totp/verify` へ送ります。
3. 応答から `twoFactorAuth` Cookieを取得し、認証済みセッションを再確認します。
4. 2つのCookieを `.env` 形式で標準出力します。

TOTPを2回行う必要はありません。`twoFactorAuth`はTOTPコードそのものではなく、TOTP検証済み端末を記憶するCookieです。

既存の `/etc/vrchat-logger.env` に、Cookie生成用の4項目を追加します。これらはメインサービスでは使用せず、Cookie生成コマンドだけが読みます。

```bash
VRC_USERNAME=YOUR_LOGIN_USERNAME
VRC_PASSWORD=YOUR_PASSWORD
VRC_TOTP_SECRET=YOUR_BASE32_TOTP_SEED
VRC_USER_AGENT="vrchat-friendnotify-auth/1.0 contact@example.com"
```

- `VRC_USERNAME` はログイン用ユーザー名です。`usr_...` 形式のVRChat User IDではありません。
- `VRC_TOTP_SECRET` は認証アプリに登録したBase32シークレットです。現在表示されている6桁コードや `otpauth://` URIではありません。
- `VRC_USER_AGENT` の連絡先は、自分が受信できるメールアドレス等へ変更してください。

オプションなしで端末から起動すると、Cookie生成後に実際の環境ファイルパスを表示し、更新するか確認します。
`y` / `yes` で更新し、`n` / `no`、空入力、EOFでは更新しません。更新した場合も断った場合もCookieは標準出力へ出るため、
標準出力は認証情報として扱ってください。質問と結果は標準エラー出力へ出るので、標準出力のパイプ形式は変わりません。

```bash
sudo go run ./cmd/vrchat-auth-cookies
```

従来どおり確認なしで同じ `/etc/vrchat-logger.env` の2つのCookie行を更新する場合は、`-update-env` を明示します。
このモードはCookieを標準出力へ表示しません。

```bash
sudo go run ./cmd/vrchat-auth-cookies -update-env
sudo systemctl restart vrchatlogs-favorites-notify.service
```

デフォルトで `/etc/vrchat-logger.env` を直接読みます。systemdの環境変数をシェルへ読み込むための `set -a` や `source` は不要です。
非TTYから起動した場合、または `-env-file` などを明示して `-interactive` を付けなかった場合は、従来どおりファイルを書き換えず、
次の2行を標準出力します。

```text
VRC_AUTH_TOKEN=authcookie_...
VRC_TWO_FACTOR_AUTH_TOKEN=...
```

別の環境ファイルで対話確認する場合は `-env-file /path/to/file -interactive` を指定します。Cookie生成を実行するたびに新しいログインセッションを作るため、
定期実行したり、サービス起動のたびに実行したりしないでください。出力Cookie、パスワード、TOTPシークレットはすべて認証情報として扱い、
Git、ログ、シェル履歴へ残さないでください。TOTPは端末時刻に依存するため、NTPで時刻同期されている必要があります。

## Sound Control API

Goプロセスは通知音のON/OFF状態を保持し、LAN内にHTTP APIを公開します。
状態変更（`toggle` / `on` / `off`）は **GET でも POST でも**叩けるため、どんな端末からでも1リクエストで操作できます。

| メソッド | パス | 動作 |
| --- | --- | --- |
| GET | `/sound` | 現在状態を返す（`{"enabled":true}`） |
| GET/POST | `/sound/toggle` | ON/OFFを反転する |
| GET/POST | `/sound/on` | ONにする |
| GET/POST | `/sound/off` | OFFにする |
| GET | `/` | スマホ等から押せる最小Webページ |

例（`<pi>` は Raspberry Pi のIP、ポートは `SOUND_API_ADDR`）:

```bash
curl http://<pi>:8099/sound              # 状態確認
curl -X POST http://<pi>:8099/sound/toggle
curl -X POST http://<pi>:8099/sound/off
```

- 状態は `runtime/sound_enabled`（`1` / `0`）に永続化され、再起動後も維持されます。
- 状態ファイルが無い場合の既定は **ON** です（ネットワーク不通時などに無音固定にならないため）。
- `toggle` / `on` / `off` の操作時は、結果の状態を**音で確認**できます：**ON=1回、OFF=2回**、通知音を鳴らします。
  - 耳で判別するための仕様です。別の音にしたい場合は `soundFile` を差し替えるか、鳴らす回数を変更してください。

## Runtime Files

実行時に以下のファイルが `runtime/` 配下に生成されます（いずれもコミット不要）。

```text
runtime/sound_enabled     通知音 ON/OFF の永続状態（1 / 0）
runtime/oled_status.txt   OLED表示内容（OLEDを使う場合のみ / レガシー）
```

推奨リポジトリ構成:

```text
vrchatlogs_favorites_notify/
├─ main.go
├─ oled_notify.py                 # 任意 / レガシー
├─ esp32/
│  └─ sound_switch/
│     └─ sound_switch.ino
├─ runtime/
│  └─ .gitkeep
├─ .env.example
├─ README.md
└─ .gitignore
```

推奨 `.gitignore`:

```gitignore
.env
*.env
runtime/*
!runtime/.gitkeep
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

## OLED Daemon（任意 / レガシー）

OLEDを接続している場合のみ使用します。GoプロセスはOLED表示内容を `runtime/oled_status.txt` に書き込み、
Python daemonがそのファイルを監視してOLEDを更新します。

手動実行例:

```bash
cd ~/vrchatlogs_favorites_notify
python3 oled_notify.py
```

手動表示テスト:

```bash
printf "ONLINE\nTestFriend\n15:04\n" > runtime/oled_status.txt
```

## systemd Setup

VRChat通知watcherサービスを設定します。OLED daemonサービスはOLEDを使う場合のみ追加します。

### Go watcher service

作成:

```bash
sudo nano /etc/systemd/system/vrchatlogs-favorites-notify.service
```

例:

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

### OLED daemon service（任意 / レガシー）

OLEDを使う場合のみ作成します。

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

反映・起動・有効化:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now vrchatlogs-favorites-notify.service
# OLEDを使う場合のみ:
# sudo systemctl enable --now vrchat-oled.service
```

ステータス / ログ:

```bash
sudo systemctl status vrchatlogs-favorites-notify.service --no-pager -l
journalctl -u vrchatlogs-favorites-notify.service -f
```

## Operation

### 通知音の ON/OFF

- ESP32-C3 のタクトスイッチを押す → トグル（LEDが現在状態を表示）
- スマホ / PC のブラウザで `http://<pi>:8099/` を開く → ON / OFF / TOGGLE
- 端末を問わず: `curl -X POST http://<pi>:8099/sound/toggle`

現在の状態の確認方法:

- 音で: 操作すると **ON=1回 / OFF=2回** 鳴ります
- 目で: ESP32のオンボードLED（点灯=ON）
- コマンド/画面で: `curl http://<pi>:8099/sound` または `http://<pi>:8099/`

### Discord

Discord通知は、通知音のON/OFF状態に関係なく送信されます。

これは、通知監視自体を止めずに、音（ローカル出力）だけをミュートできるようにするための意図的な仕様です。

## Troubleshooting

### 音が鳴らない

まず音声ファイルとALSAを確認します。

```bash
ls -l /home/satomi/notify.wav
aplay /home/satomi/notify.wav
```

次に通知音の状態を確認します。

```bash
curl http://<pi>:8099/sound     # {"enabled":true} になっているか
cat runtime/sound_enabled       # 1 なら ON
```

`{"enabled":false}` の場合は OFF です。ON に戻します。

```bash
curl -X POST http://<pi>:8099/sound/on
```

### ESP32がWi-Fiに繋がらない（スキャンはできるのに認証で失敗する）

シリアルに `wifi disconnected, reason=2` が延々と出て、**どのAPに対しても**（スマホのテザリング相手でも）接続できない場合、
C3 SuperMiniによくある**アンテナ整合回路の不良個体**の可能性が高いです。受信（スキャン）は正常でも、
最大出力の送信が歪んでAPに認証フレームが届かず、無視され続けます。

本ファームウェアは対策として送信出力を下げています（`WiFi.setTxPower(WIFI_POWER_8_5dBm)` in `connectWiFi()`）。
それでも繋がらない場合は、さらに低い値（`WIFI_POWER_5dBm` / `WIFI_POWER_2dBm`）も試してください。
良品基板で電波の届きが悪い場合は、この行を外して出力を上げることもできます。

- `reason=2` = APが認証に応答していない（電波・個体不良・MACフィルタ）
- `reason=15` = パスワード間違い（4-wayハンドシェイク失敗）
- `reason=201` = SSIDが見つからない（5GHz専用SSID・圏外）

### ESP32からトグルできない

- ESP32のシリアルモニタ（115200）でWi-Fi接続とHTTP応答コードを確認します。
- `PI_HOST` / `PI_PORT` が Raspberry Pi のIPと `SOUND_API_ADDR` に一致しているか確認します。
- Raspberry Pi 上でAPIが待ち受けているか確認します。

```bash
curl http://<pi>:8099/sound
sudo ss -ltnp | grep 8099
```

### 環境変数が読み込まれない

Goサービスファイルに以下が含まれていることを確認します。

```ini
EnvironmentFile=/etc/vrchat-logger.env
```

サービスログを確認します。

```bash
journalctl -u vrchatlogs-favorites-notify.service -n 80 --no-pager
```

以下のような表示がある場合、環境変数ファイルが存在しない / 読み取れない / 設定が間違っています。

```text
VRC_AUTH_TOKEN not found
VRC_TWO_FACTOR_AUTH_TOKEN not found
```

### お気に入りリストを取得できない

ログに以下が含まれる場合:

```text
Requires Two-Factor Authentication
```

`VRC_TWO_FACTOR_AUTH_TOKEN` が存在しない、期限切れ、または間違っています。
ログイン済みのブラウザセッションから新しい `twoFactorAuth` cookie を取得して `/etc/vrchat-logger.env` を更新し、サービスを再起動します。

```bash
sudo systemctl restart vrchatlogs-favorites-notify.service
```

### OLEDに何も表示されない（任意 / レガシー）

```bash
sudo i2cdetect -y 1                         # 3c が見えるか
sudo systemctl restart vrchat-oled.service
journalctl -u vrchat-oled.service -n 80 --no-pager
```

## Security Notes

* 以下のような機密情報は絶対にコミットしないでください。

  * `VRC_AUTH_TOKEN`
  * `VRC_TWO_FACTOR_AUTH_TOKEN`
  * `DISCORD_WEBHOOK_URL_FAVORITES`
* WebSocket接続では、auth tokenを含むURLを使用します。
  `wss://pipeline.vrchat.cloud/?authToken=...`
* 通知音制御API（`SOUND_API_ADDR`）は認証を持ちません。**信頼できるLAN内でのみ**公開し、インターネットに晒さないでください。
* ログ、スクリーンショット、シェル履歴、監視ツール、クラッシュレポートなどからトークンが漏れないよう注意してください。
* トークンまたはWebhook URLが漏えいした疑いがある場合は、ただちにローテーションしてください。
* `/etc/vrchat-logger.env` はrootのみが読み取れるようにしてください。

## Disclaimer

自己責任で使用してください。VRChat、Discord、および関連するサービスのルールや利用規約に従って使用してください。
