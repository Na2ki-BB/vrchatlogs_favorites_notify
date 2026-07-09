package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36"

// soundEnabled is the single source of truth for whether notifications play a
// sound. It is toggled over HTTP (from an ESP32 button, a phone, curl, ...) and
// persisted to disk so the choice survives restarts.
var soundEnabled atomic.Bool

const soundStateFile = "runtime/sound_enabled"

var hubDataDir string

type envelope struct {
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content"`
}

type FriendOnline struct {
	UserID           string `json:"userId"`
	Platform         string `json:"platform"`
	Location         string `json:"location"`
	CanRequestInvite bool   `json:"canRequestInvite"`
}

type FriendOffline struct {
	UserID   string `json:"userId"`
	Platform string `json:"platform"`
}

type FriendLocation struct {
	UserID              string `json:"userId"`
	Location            string `json:"location"`
	TravelingToLocation string `json:"travelingToLocation"`
	WorldID             string `json:"worldId"`
	CanRequestInvite    bool   `json:"canRequestInvite"`
}

type Favorite struct {
	FavoriteID string   `json:"favoriteId"`
	ID         string   `json:"id"`
	Tags       []string `json:"tags"`
	Type       string   `json:"type"`
}

func notifyDiscord(webhookURL, msg string) {
	if webhookURL == "" {
		return
	}

	payload := map[string]string{
		"content": msg,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		log.Println("discord marshal error:", err)
		return
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(b))
	if err != nil {
		log.Println("discord req error:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		log.Println("discord post error:", err)
		return
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		log.Printf("discord non-2xx: %d body=%s\n", res.StatusCode, string(body))
	}
}

func notifyOnlineStatus(webhookURL, status, userName string) {
	now := time.Now().Format("15:04")
	msg := fmt.Sprintf("%s\n%s\n%s", status, userName, now)

	log.Printf("[STATUS] %s user=%s\n", status, userName)
	notifyDiscord(webhookURL, msg)

	publishStatus(status, userName, now, "")

	if isSoundEnabled() {
		playSound()
	}
}

func notifyWorldMove(webhookURL, userName, worldName string) {
	destination := worldName

	if destination == "" {
		destination = "unknown"
	}

	now := time.Now().Format("15:04")
	msg := fmt.Sprintf("MOVE\n%s\n%s\n%s", userName, destination, now)

	log.Printf("[WORLD MOVE] user=%s destination=%s\n", userName, destination)
	notifyDiscord(webhookURL, msg)

	publishStatus("MOVE", userName, destination, now)

	if isSoundEnabled() {
		playSound()
	}
}

func fetchFavoriteFriendIDs(token, twoFactorToken, tag string) (map[string]bool, error) {
	params := url.Values{}
	params.Set("type", "friend")
	params.Set("tag", tag)
	params.Set("n", "100")

	apiURL := "https://api.vrchat.cloud/api/1/favorites?" + params.Encode()

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("favorites req error: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.AddCookie(&http.Cookie{
		Name:  "auth",
		Value: token,
	})
	req.AddCookie(&http.Cookie{
		Name:  "twoFactorAuth",
		Value: twoFactorToken,
	})

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("favorites res error: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("favorites read error: %w", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("favorites non-2xx: %d body=%s", res.StatusCode, string(body))
	}

	var favorites []Favorite
	if err := json.Unmarshal(body, &favorites); err != nil {
		return nil, fmt.Errorf("favorites unmarshal error: %w", err)
	}

	targets := map[string]bool{}

	for _, fav := range favorites {
		if fav.Type != "friend" {
			continue
		}
		if fav.FavoriteID == "" {
			continue
		}

		targets[fav.FavoriteID] = true
	}

	return targets, nil
}
func getUserDisplayName(token, userID string) string {
	apiURL := "https://api.vrchat.cloud/api/1/users/" + userID

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Println("userName req error:", err)
		return ""
	}

	req.Header.Set("User-Agent", userAgent)
	req.AddCookie(&http.Cookie{
		Name:  "auth",
		Value: token,
	})

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		log.Println("userName res error:", err)
		return ""
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Println("userName read error:", err)
		return ""
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		log.Printf("userName non-2xx: %d body=%s\n", res.StatusCode, string(body))
		return ""
	}

	var parsed struct {
		DisplayName string `json:"displayName"`
	}

	if err := json.Unmarshal(body, &parsed); err != nil {
		log.Println("userName unmarshal error:", err)
		return ""
	}

	return parsed.DisplayName
}
func getWorldName(token, worldID string) string {
	if worldID == "private" {
		return "private"
	}
	apiURL := "https://api.vrchat.cloud/api/1/worlds/" + worldID

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Println("worldName req error:", err)
		return ""
	}
	req.Header.Set("User-Agent", userAgent)
	req.AddCookie(&http.Cookie{
		Name:  "auth",
		Value: token,
	})

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		log.Println("worldName res error:", err)
		return ""
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Println("worldName read error:", err)
		return ""
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		log.Printf("worldName non-2xx: %d body=%s\n", res.StatusCode, string(body))
		return ""
	}

	var parsed struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		log.Println("worldName unmarshal error:", err)
		return ""
	}
	return parsed.Name
}

// publishStatus pushes the latest notification lines to the optional hub
// integration (HUB_DATA_DIR). The physical OLED has been retired, so this is
// now the only status sink besides Discord.
func publishStatus(line1, line2, line3, line4 string) {
	writeHubJSON(line1, line2, line3, line4)
}

func writeHubJSON(line1, line2, line3, line4 string) {
	if hubDataDir == "" {
		return
	}

	data := map[string]interface{}{
		"updated_at": time.Now().Format("2006-01-02T15:04:05"),
		"lines":      []string{line1, line2, line3, line4},
	}

	b, err := json.Marshal(data)
	if err != nil {
		log.Println("hub json marshal error:", err)
		return
	}

	tmp := hubDataDir + "/vrchat.json.tmp"
	dst := hubDataDir + "/vrchat.json"

	if err := os.WriteFile(tmp, b, 0644); err != nil {
		log.Println("hub json write error:", err)
		return
	}

	if err := os.Rename(tmp, dst); err != nil {
		log.Println("hub json rename error:", err)
	}
}

func playSound() {
	go func() {
		cmd := exec.Command("aplay", "/home/satomi/notify.wav")

		if err := cmd.Run(); err != nil {
			log.Println("sound play error:", err)
		}
	}()
}

func isSoundEnabled() bool {
	return soundEnabled.Load()
}

// setSoundEnabled updates the state and persists it. It returns true when the
// value actually changed.
func setSoundEnabled(enabled bool) (changed bool) {
	changed = soundEnabled.Swap(enabled) != enabled
	if changed {
		persistSoundState()
		log.Printf("sound enabled -> %v\n", enabled)
	}
	return changed
}

// loadSoundState restores the persisted flag. Defaults to ON so a fresh install
// (or a lost state file) still notifies audibly.
func loadSoundState() {
	soundEnabled.Store(true)

	b, err := os.ReadFile(soundStateFile)
	if err != nil {
		return
	}

	switch strings.TrimSpace(string(b)) {
	case "0", "false", "off":
		soundEnabled.Store(false)
	default:
		soundEnabled.Store(true)
	}
}

func persistSoundState() {
	if err := os.MkdirAll("runtime", 0755); err != nil {
		log.Println("sound state mkdir error:", err)
		return
	}

	v := "1"
	if !soundEnabled.Load() {
		v = "0"
	}

	if err := os.WriteFile(soundStateFile, []byte(v+"\n"), 0644); err != nil {
		log.Println("sound state write error:", err)
	}
}

const controlIndexHTML = `<!doctype html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>VRChat Notify Sound</title>
<style>
  body{font-family:system-ui,sans-serif;max-width:24rem;margin:3rem auto;padding:0 1rem;text-align:center}
  #state{font-size:2rem;margin:1.5rem 0;font-weight:700}
  button{font-size:1.1rem;padding:.8rem 1.4rem;margin:.3rem;border:0;border-radius:.6rem;cursor:pointer}
  .on{background:#2e7d32;color:#fff}.off{background:#c62828;color:#fff}.tg{background:#455a64;color:#fff}
</style>
</head>
<body>
  <h1>通知音</h1>
  <div id="state">...</div>
  <button class="on" onclick="send('/sound/on')">ON</button>
  <button class="off" onclick="send('/sound/off')">OFF</button>
  <button class="tg" onclick="send('/sound/toggle')">TOGGLE</button>
<script>
async function refresh(){const r=await fetch('/sound');const j=await r.json();render(j.enabled);}
async function send(p){const r=await fetch(p,{method:'POST'});const j=await r.json();render(j.enabled);}
function render(on){document.getElementById('state').textContent=on?'🔊 ON':'🔇 OFF';}
refresh();
</script>
</body>
</html>`

// startControlServer exposes the sound state over HTTP so any device can flip
// it. Mutations respond to GET or POST alike, so anything from an ESP32 to a
// browser bookmark to `curl` works with a single request.
func startControlServer(addr string) {
	mux := http.NewServeMux()

	writeState := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"enabled": isSoundEnabled()})
	}

	mutate := func(w http.ResponseWriter, next func(cur bool) bool) {
		target := next(isSoundEnabled())
		changed := setSoundEnabled(target)
		// Play a short confirmation only when sound transitions to ON.
		if target && changed {
			playSound()
		}
		writeState(w)
	}

	mux.HandleFunc("/sound/toggle", func(w http.ResponseWriter, r *http.Request) {
		mutate(w, func(cur bool) bool { return !cur })
	})
	mux.HandleFunc("/sound/on", func(w http.ResponseWriter, r *http.Request) {
		mutate(w, func(bool) bool { return true })
	})
	mux.HandleFunc("/sound/off", func(w http.ResponseWriter, r *http.Request) {
		mutate(w, func(bool) bool { return false })
	})
	mux.HandleFunc("/sound", func(w http.ResponseWriter, r *http.Request) {
		writeState(w)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, controlIndexHTML)
	})

	go func() {
		log.Printf("sound control server listening on %s\n", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Println("control server error:", err)
		}
	}()
}

func main() {
	token := os.Getenv("VRC_AUTH_TOKEN")
	if token == "" {
		log.Fatal("VRC_AUTH_TOKEN not found")
	}
	twoFactorToken := os.Getenv("VRC_TWO_FACTOR_AUTH_TOKEN")
	if twoFactorToken == "" {
		log.Fatal("VRC_TWO_FACTOR_AUTH_TOKEN not found")
	}
	favoriteTag := os.Getenv("VRC_NOTIFY_FAVORITE_TAG")
	if favoriteTag == "" {
		favoriteTag = "group_0"
	}

	hubDataDir = os.Getenv("HUB_DATA_DIR")
	discordWebhook := os.Getenv("DISCORD_WEBHOOK_URL_FAVORITES")

	controlAddr := os.Getenv("SOUND_API_ADDR")
	if controlAddr == "" {
		controlAddr = ":8099"
	}

	loadSoundState()
	log.Printf("sound initial state: enabled=%v\n", isSoundEnabled())
	startControlServer(controlAddr)

	var targetMu sync.RWMutex
	targetFriendIDs := map[string]bool{}
	userNameMap := map[string]string{}

	refreshTargets := func() {
		targets, err := fetchFavoriteFriendIDs(token, twoFactorToken, favoriteTag)
		if err != nil {
			log.Println("favorite refresh error:", err)
			return
		}

		newUserNameMap := map[string]string{}

		for userID := range targets {
			displayName := getUserDisplayName(token, userID)
			if displayName == "" {
				displayName = userID
			}

			newUserNameMap[userID] = displayName
		}

		targetMu.Lock()
		targetFriendIDs = targets
		userNameMap = newUserNameMap
		targetMu.Unlock()

		log.Printf("favorite targets refreshed: tag=%s count=%d\n", favoriteTag, len(targets))
	}

	isTargetFriend := func(userID string) bool {
		targetMu.RLock()
		defer targetMu.RUnlock()

		return targetFriendIDs[userID]
	}

	getDisplayName := func(userID string) string {
		targetMu.RLock()
		defer targetMu.RUnlock()

		displayName := userNameMap[userID]
		if displayName == "" {
			return userID
		}

		return displayName
	}
	refreshTargets()

	publishStatus("VRChat Notify", "Waiting...", time.Now().Format("15:04"), "")

	if isSoundEnabled() {
		playSound()
	}

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			refreshTargets()
		}
	}()

	wsURL := "wss://pipeline.vrchat.cloud/?authToken=" + token

	h := http.Header{}
	h.Set("User-Agent", userAgent)

	for {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, h)
		if err != nil {
			log.Println("dial error:", err)
			time.Sleep(3 * time.Second)
			continue
		}

		log.Println("Connected!")

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("read error:", err)
				break
			}

			var env envelope
			if err := json.Unmarshal(message, &env); err != nil {
				log.Println("envelope unmarshal error:", err)
				continue
			}

			content := env.Content

			var s string
			if err := json.Unmarshal(env.Content, &s); err == nil && len(s) > 0 && s[0] == '{' {
				content = json.RawMessage(s)
			}

			switch env.Type {
			case "friend-online":
				var v FriendOnline
				if err := json.Unmarshal(content, &v); err != nil {
					log.Println("friend-online unmarshal error:", err)
					continue
				}

				if isTargetFriend(v.UserID) {
					displayName := getDisplayName(v.UserID)
					notifyOnlineStatus(discordWebhook, "ONLINE", displayName)
				}

			case "friend-offline":
				var v FriendOffline
				if err := json.Unmarshal(content, &v); err != nil {
					log.Println("friend-offline unmarshal error:", err)
					continue
				}

				if isTargetFriend(v.UserID) {
					displayName := getDisplayName(v.UserID)
					notifyOnlineStatus(discordWebhook, "OFFLINE", displayName)
				}

			case "friend-location":
				var v FriendLocation
				if err := json.Unmarshal(content, &v); err != nil {
					log.Println("friend-location unmarshal error:", err)
					continue
				}

				if isTargetFriend(v.UserID) {
					name := getWorldName(token, v.WorldID)

					displayName := getDisplayName(v.UserID)

					notifyWorldMove(
						discordWebhook,
						displayName,
						name,
					)
				}
			}
		}

		_ = conn.Close()
		time.Sleep(2 * time.Second)
		log.Println("Reconnecting...")
	}
}
