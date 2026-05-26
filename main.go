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
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/host/v3"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36"

var displayPin gpio.PinIO
var soundPin gpio.PinIO
var oledMu sync.Mutex
var latestOLEDLines = [4]string{"VRChat Notify", "Waiting...", "", ""}

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

	showOLED(status, userName, now, "")

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

	showOLED("MOVE", userName, destination, now)

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

func writeOLEDFile(line1, line2, line3, line4 string) {
	if err := os.MkdirAll("runtime", 0755); err != nil {
		log.Println("oled mkdir error:", err)
		return
	}

	content := fmt.Sprintf("%s\n%s\n%s\n%s\n", line1, line2, line3, line4)

	if err := os.WriteFile("runtime/oled_status.txt", []byte(content), 0644); err != nil {
		log.Println("oled write error:", err)
	}
}

func showOLED(line1, line2, line3, line4 string) {
	oledMu.Lock()
	latestOLEDLines = [4]string{line1, line2, line3, line4}
	oledMu.Unlock()

	if isDisplayEnabled() {
		writeOLEDFile(line1, line2, line3, line4)
	}
}

func restoreOLED() {
	oledMu.Lock()
	lines := latestOLEDLines
	oledMu.Unlock()

	writeOLEDFile(lines[0], lines[1], lines[2], lines[3])
}

func clearOLED() {
	writeOLEDFile("", "", "", "")
}
func playSound() {
	cmd := exec.Command("aplay", "/home/satomi/notify.wav")

	if err := cmd.Start(); err != nil {
		log.Println("sound play error:", err)
	}
}

func initGPIO() error {
	if _, err := host.Init(); err != nil {
		return err
	}

	displayPin = gpioreg.ByName("GPIO17")
	if displayPin == nil {
		return fmt.Errorf("GPIO17 not found")
	}

	soundPin = gpioreg.ByName("GPIO27")
	if soundPin == nil {
		return fmt.Errorf("GPIO27 not found")
	}

	if err := displayPin.In(gpio.PullUp, gpio.NoEdge); err != nil {
		return fmt.Errorf("GPIO17 input error: %w", err)
	}

	if err := soundPin.In(gpio.PullUp, gpio.NoEdge); err != nil {
		return fmt.Errorf("GPIO27 input error: %w", err)
	}

	return nil
}

func isDisplayEnabled() bool {
	if displayPin == nil {
		return true
	}

	return displayPin.Read() == gpio.Low
}

func isSoundEnabled() bool {
	if soundPin == nil {
		return true
	}

	return soundPin.Read() == gpio.Low
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

	discordWebhook := os.Getenv("DISCORD_WEBHOOK_URL_FAVORITES")

	if err := initGPIO(); err != nil {
		log.Fatal("gpio init error:", err)
	}

	log.Printf("switch initial state: display=%v sound=%v\n", isDisplayEnabled(), isSoundEnabled())
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

	go func() {
		lastDisplayEnabled := isDisplayEnabled()

		for {
			current := isDisplayEnabled()

			if lastDisplayEnabled && !current {
				clearOLED()
				log.Println("display disabled: OLED cleared")
			}

			if !lastDisplayEnabled && current {
				restoreOLED()
				log.Println("display enabled: OLED restored")
			}

			lastDisplayEnabled = current
			time.Sleep(300 * time.Millisecond)
		}
	}()

	if isDisplayEnabled() {
		showOLED("TEST", "Sound", time.Now().Format("15:04"), "")
	} else {
		clearOLED()
	}

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
