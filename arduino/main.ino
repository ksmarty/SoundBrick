#include <Arduino.h>
#include <AsyncElegantOTA.h>
// #include <ESP8266Ping.h>
#include <AsyncPing.h>
#include <ESP8266WiFi.h>
#include <ESPAsyncTCP.h>
#include <ESPAsyncWebServer.h>
#include <ESPConnect.h>
#include <WiFiUdp.h>

#define UDP_RECEIVE_PORT 4210
#define UDP_SEND_PORT 4211

#define STAT1 5
#define STAT2 4
#define STAT3 0
#define STAT4 2

#define SEL1 16
#define SEL2 14
#define MUTE 12

#define HEARTBEAT_TIMEOUT_MS 10000

AsyncWebServer server(80);
DNSServer dns;

AsyncPing ping;
WiFiUDP UDP;
char packet[255];

int currentOutput;
unsigned long time_now = 0;

class StatusPin {
   public:
    StatusPin(int pin) {
        this->pin = pin;
        pinMode(pin, OUTPUT);
        this->set(LOW);
    }
    void set(bool state) {
        digitalWrite(pin, state);
    }

   private:
    int pin;
};

StatusPin statusPins[4] = {{STAT1}, {STAT2}, {STAT3}, {STAT4}};

enum class Status {
    OUT1 = 0,
    OUT2 = 1,
    OUT3 = 2,
    OUT4 = 3,
    Muted = 4,
    Error = -1,
    ClientCheck = -2,
};

void allStatus(bool state) {
    for (StatusPin n : statusPins) n.set(state);
}

bool isMuted() {
    return digitalRead(MUTE);
}

/**
 * Set mute state
 * @param state Use boolean to set state. If unset, will toggle.
 * @return 0 - Unmuted, 1 - muted
 */
bool mute(int state = -1) {
    // Invert current state or use provided
    int newState = (state == -1) ? !isMuted() : state;

    digitalWrite(MUTE, newState);

    return newState;
}

Status toStatus(int x) {
    try {
        return static_cast<Status>(x);
    } catch (const std::exception &e) {
        return Status::Error;
    }
}

Status getCurrentOutput() {
    return toStatus(currentOutput);
}

Status setOutput(int output) {
    Status request = toStatus(output);

    switch (request) {
        case Status::ClientCheck:
            return isMuted() ? Status::Muted : getCurrentOutput();
        case Status::Muted:
            if (mute()) return Status::Muted;
        default:
            if (isMuted()) return Status::Error;
    }

    currentOutput = output;

    mute(true);
    digitalWrite(SEL1, (currentOutput >> 0) & 1);
    digitalWrite(SEL2, (currentOutput >> 1) & 1);
    mute(false);

    return getCurrentOutput();
}

Status managePacket(char *packet) {
    int x;

    try {
        x = atoi(packet);
    } catch (const std::exception &e) {
        x = -1;
    }

    return setOutput(x);
}

void OTA() {
    server.on("/", HTTP_GET, [](AsyncWebServerRequest *request) {
        request->redirect("/update");
    });

    AsyncElegantOTA.begin(&server);
    server.begin();
}

void setupHeartbeat() {
    ping.on(true, [](const AsyncPingResponse &response) {
        allStatus(LOW);
        if (response.answer)
            statusPins[currentOutput].set(HIGH);
        return true;
    });
}

void heartbeat() {
    time_now = millis();
    if (!UDP.remoteIP().isSet()) return;
    ping.begin(UDP.remoteIP());
}

void setup() {
    Serial.begin(115200);
    Serial.println();

    for (const int &n : {SEL1, SEL2, MUTE}) {
        pinMode(n, OUTPUT);
    }
    setOutput(0);

    ESPConnect.autoConnect("SoundSwitch-Setup");
    ESPConnect.begin(&server);

    allStatus(HIGH);
    Serial.println("\nConnected!");
    Serial.print("IP address: ");
    Serial.println(WiFi.localIP());

    OTA();
    setupHeartbeat();

    UDP.begin(UDP_RECEIVE_PORT);

    Serial.println("Device ready!");
}

void loop() {
    if (millis() > time_now + HEARTBEAT_TIMEOUT_MS) heartbeat();

    if (!UDP.parsePacket()) return;

    int len = UDP.read(packet, 255);
    if (len > 0) packet[len] = '\0';

    Serial.printf("Packet received: ", len);
    Serial.println(packet);

    int val = static_cast<int>(managePacket(packet));
    char buf[8];
    itoa(val, buf, 10);

    Serial.printf("Sending Packet: %d\n", val);
    UDP.beginPacket(UDP.remoteIP(), UDP_SEND_PORT);
    UDP.write(buf);
    UDP.endPacket();
}