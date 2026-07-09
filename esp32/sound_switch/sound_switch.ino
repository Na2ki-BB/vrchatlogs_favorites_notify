// VRChat Notify - sound on/off remote for ESP32-C3 SuperMini
//
// This is only the sound-remote portion; the board is meant to host other
// features too, so it uses a dedicated tact switch (not the BOOT button) and
// leaves BOOT/UART pins free.
//
// The ESP32 is intentionally "dumb": it holds no state. Pressing the tact
// switch fires a single HTTP request to the Raspberry Pi, which owns the
// authoritative sound on/off flag. The onboard LED simply mirrors whatever the
// Pi reports back, so there is no local state to keep in sync.
//
// Board: "ESP32C3 Dev Module" (Arduino core for ESP32).
//   Wiring:
//   - tact switch : between GPIO4 and GND (internal pull-up, pressed = LOW)
//   - onboard LED  : GPIO8 (active LOW on most C3 SuperMini boards) -> status
//   GPIO8/GPIO9 are strapping pins; GPIO20/21 are UART. GPIO4 avoids all of
//   them. Change PIN_BUTTON below if you need that pin for another feature.
//
// Endpoints on the Pi (see main.go):
//   POST /sound/toggle -> {"enabled":true|false}
//   GET  /sound        -> {"enabled":true|false}

#include <WiFi.h>
#include <HTTPClient.h>

// Wi-Fi + Raspberry Pi config lives in secrets.h (gitignored). Copy
// secrets.h.example to secrets.h and fill in your real values.
#include "secrets.h"  // WIFI_SSID / WIFI_PASS / PI_HOST / PI_PORT

// ---- pins -----------------------------------------------------------------
static const int PIN_BUTTON = 4;   // external tact switch to GND
static const int PIN_LED    = 8;   // onboard LED (status)
static const bool LED_ACTIVE_LOW = true;

static int  lastButton   = HIGH;   // pull-up idle = HIGH
static unsigned long lastEdgeMs = 0;
static const unsigned long DEBOUNCE_MS = 40;

void setLed(bool on) {
  digitalWrite(PIN_LED, (on == LED_ACTIVE_LOW) ? LOW : HIGH);
}

// onWiFiEvent surfaces the disconnect reason, which makes failures obvious:
//   reason=2 or 15 -> wrong password, reason=201 -> SSID not found/out of range.
void onWiFiEvent(WiFiEvent_t event, WiFiEventInfo_t info) {
  if (event == ARDUINO_EVENT_WIFI_STA_DISCONNECTED) {
    Serial.printf("wifi disconnected, reason=%d\n", info.wifi_sta_disconnected.reason);
  }
}

void connectWiFi() {
  WiFi.onEvent(onWiFiEvent);
  WiFi.mode(WIFI_STA);
  Serial.printf("connecting to \"%s\"\n", WIFI_SSID);
  WiFi.begin(WIFI_SSID, WIFI_PASS);

  unsigned long start = millis();
  while (WiFi.status() != WL_CONNECTED) {
    delay(500);
    Serial.print(".");
    setLed(!digitalRead(PIN_LED)); // blink while connecting
    if (millis() - start > 20000) {
      Serial.println("\ntimeout - retrying...");
      WiFi.disconnect(true);
      delay(200);
      WiFi.begin(WIFI_SSID, WIFI_PASS);
      start = millis();
    }
  }
  Serial.printf("\nconnected: %s\n", WiFi.localIP().toString().c_str());
}

// request hits the given path and returns the reported enabled state.
// `ok` is set false on any network/parse failure so the caller can leave the
// LED untouched rather than lie about the state.
bool requestState(const char *path, const char *method, bool &ok) {
  ok = false;
  if (WiFi.status() != WL_CONNECTED) {
    connectWiFi();
  }

  HTTPClient http;
  String url = String("http://") + PI_HOST + ":" + PI_PORT + path;
  if (!http.begin(url)) {
    Serial.println("http begin failed");
    return false;
  }

  int code = (strcmp(method, "POST") == 0) ? http.POST("") : http.GET();
  if (code != 200) {
    Serial.printf("http %s %s -> %d\n", method, path, code);
    http.end();
    return false;
  }

  String body = http.getString();
  http.end();

  // minimal parse, no JSON library needed
  bool enabled = body.indexOf("\"enabled\":true") >= 0;
  Serial.printf("%s -> %s\n", path, enabled ? "ON" : "OFF");
  ok = true;
  return enabled;
}

void setup() {
  Serial.begin(115200);
  pinMode(PIN_LED, OUTPUT);
  pinMode(PIN_BUTTON, INPUT_PULLUP);
  setLed(false);

  connectWiFi();

  // sync LED to current state on boot
  bool ok;
  bool state = requestState("/sound", "GET", ok);
  if (ok) setLed(state);
}

void loop() {
  int reading = digitalRead(PIN_BUTTON);

  if (reading != lastButton) {
    lastEdgeMs = millis();
    lastButton = reading;
  }

  // falling edge (button pressed) after debounce window
  static bool handled = false;
  if (reading == LOW && !handled && (millis() - lastEdgeMs) > DEBOUNCE_MS) {
    handled = true;
    bool ok;
    bool state = requestState("/sound/toggle", "POST", ok);
    if (ok) setLed(state);
  }
  if (reading == HIGH) {
    handled = false;
  }

  delay(10);
}
