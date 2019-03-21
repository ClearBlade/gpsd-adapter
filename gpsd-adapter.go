package main

import (
	"encoding/json"
	"flag"
	cb "github.com/clearblade/Go-SDK"
	mqtt "github.com/clearblade/paho.mqtt.golang"
	"github.com/hashicorp/logutils"
	"github.com/stratoberry/go-gpsd"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	msgPublishQOS      = 0
	gpsdDataTopic      = "<topic_root>/gpsd-data"
	defaultTopicRoot   = "gpsd"
	defaultGpsdAddress = "localhost:2947"
)

var (
	sysKey              string
	sysSec              string
	deviceName          string
	activeKey           string
	platformURL         string
	messagingURL        string
	logLevel            string
	adapterConfigCollID string
	cbClient            *cb.DeviceClient
	config              adapterConfig
)

type adapterSettings struct {
	GpsdAddress string `json:"gpsd_address"`
}

type adapterConfig struct {
	AdapterSettings adapterSettings `json:"adapter_settings"`
	TopicRoot       string          `json:"topic_root"`
}

func init() {
	flag.StringVar(&sysKey, "systemKey", "", "system key (required)")
	flag.StringVar(&sysSec, "systemSecret", "", "system secret (required)")
	flag.StringVar(&deviceName, "deviceName", "gpsdAdapter", "name of device (optional)")
	flag.StringVar(&activeKey, "activeKey", "", "active key for device authentication (required)")
	flag.StringVar(&platformURL, "platformURL", "http://localhost:9000", "platform url (optional)")
	flag.StringVar(&messagingURL, "messagingURL", "localhost:1883", "messaging URL (optional)")
	flag.StringVar(&logLevel, "logLevel", "info", "The level of logging to use. Available levels are 'debug, 'info', 'warn', 'error', 'fatal' (optional)")
	flag.StringVar(&adapterConfigCollID, "adapterConfigCollectionID", "", "The ID of the data collection used to house adapter configuration (required)")
}

func usage() {
	log.Printf("Usage: gpsdAdapter [options]\n\n")
	flag.PrintDefaults()
}

func validateFlags() {
	flag.Parse()

	if sysKey == "" || sysSec == "" || activeKey == "" || adapterConfigCollID == "" {
		log.Println("ERROR - Missing required flags")
		flag.Usage()
		os.Exit(1)
	}
}

func main() {

	log.Println("Starting gpsdAdapter...")

	flag.Usage = usage
	validateFlags()

	rand.Seed(time.Now().UnixNano())

	logfile, err := os.OpenFile("/var/log/gpsdAdapter", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %s", err.Error())
	}
	defer logfile.Close()
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"},
		MinLevel: logutils.LogLevel(strings.ToUpper(logLevel)),
		Writer:   logfile,
	}
	log.SetOutput(filter)

	initClearBlade()
	initAdapterConfig()
	connectClearBlade()

	log.Println("[DEBUG] main - starting info log ticker")

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			log.Printf("[INFO] Reading gpsd data...")
		}
	}

}

func initClearBlade() {
	log.Println("[DEBUG] initClearBlade - initializing clearblade")
	cbClient = cb.NewDeviceClientWithAddrs(platformURL, messagingURL, sysKey, sysSec, deviceName, activeKey)
	for err := cbClient.Authenticate(); err != nil; {
		log.Printf("[ERROR] initClearBlade - Error authenticating %s: %s\n", deviceName, err.Error())
		log.Println("[INFO] initClearBlade - Trying again in 1 minute...")
		time.Sleep(time.Minute * 1)
		err = cbClient.Authenticate()
	}
	log.Println("[INFO] initClearBlade - clearblade successfully initialized")
}

func initAdapterConfig() {
	config = adapterConfig{TopicRoot: defaultTopicRoot, AdapterSettings: adapterSettings{GpsdAddress: defaultGpsdAddress}}
	log.Println("[DEBUG] initAdapterConfig - loading adapter config from collection")
	query := cb.NewQuery()
	query.EqualTo("adapter_name", "gpsdAdapter")
	results, err := cbClient.GetData(adapterConfigCollID, query)
	if err != nil {
		log.Printf("[ERROR] initAdapterConfig - failed to fetch adapter config: %s\n", err.Error())
		log.Printf("[INFO] initAdapterConfig - using default adapter config: %+v\n", config)
	} else {
		data := results["DATA"].([]interface{})
		if len(data) == 1 {
			configData := data[0].(map[string]interface{})
			log.Printf("[DEBUG] initAdapterConfig - fetched config:\n%+v\n", configData)
			if configData["topic_root"] != nil {
				config.TopicRoot = configData["topic_root"].(string)
			} else {
				log.Printf("[INFO] initAdapterConfig - topic_root is nil, using default adapter config: %+v\n", config)
			}
			if configData["adapter_settings"] != nil {
				var aS adapterSettings
				if err := json.Unmarshal([]byte(configData["adapter_settings"].(string)), &aS); err != nil {
					log.Printf("[ERROR] initAdapterConfig - failed to unmarshal adapter_settings: %s\n", err.Error())
				}
				config.AdapterSettings = aS
			} else {
				log.Printf("[INFO] initAdapterConfig - adapter settings is nil, using default settings: %+v\n", config.AdapterSettings)
			}
		} else {
			log.Printf("[ERROR] initAdapterConfig - unexpected number of matching adapter configs: %d\n", len(data))
			log.Printf("[INFO] initAdapterConfig - using default adapter config: %+v\n", config)
		}
	}
	log.Println("[INFO] initAdapterConfig - adapter config successfully loaded")
}

func connectClearBlade() {
	log.Println("[INFO] connectClearBlade - connecting ClearBlade MQTT")
	callbacks := cb.Callbacks{OnConnectCallback: onConnect, OnConnectionLostCallback: onConnectLost}
	if err := cbClient.InitializeMQTTWithCallback(deviceName+"-"+strconv.Itoa(rand.Intn(10000)), "", 30, nil, nil, &callbacks); err != nil {
		log.Fatalf("[FATAL] connectClearBlade - Unable to connect ClearBlade MQTT: %s", err.Error())
	}
}

func onConnect(client mqtt.Client) {
	log.Println("[INFO] onConnect - ClearBlade MQTT successfully connected")
	go readGpsdData()
}

func onConnectLost(client mqtt.Client, connerr error) {
	log.Printf("[ERROR] onConnectLost - ClearBlade MQTT lost connection: %s", connerr.Error())
	// reconnect logic should be handled by go/paho sdk under the covers
}

func readGpsdData() {
	log.Println("[INFO] readGpsdData - creating gpsd connection")
	gps, err := gpsd.Dial(config.AdapterSettings.GpsdAddress)
	if err != nil {
		log.Fatalf("[FATAL] readGpsdData - faled to connect to gpsd: %s", err.Error())
	}
	gps.AddFilter("TPV", handleGpsdTPVReport)
	log.Println("[INFO] readGpsdData - starting to read gpsd data")
	_ = gps.Watch()
}

func handleGpsdTPVReport(r interface{}) {
	report := r.(*gpsd.TPVReport)
	b, err := json.Marshal(report)
	if err != nil {
		log.Printf("[ERROR] handleGpsdTPVReport - failed to marshal TPV report: %s\n", err.Error())
		return
	}
	if err := cbClient.Publish(strings.Replace(gpsdDataTopic, "<topic_root>", config.TopicRoot, 1), b, msgPublishQOS); err != nil {
		log.Printf("[ERROR] handleGpsdTPVReport - failed to publish message: %s\n", err.Error())
	}
}
