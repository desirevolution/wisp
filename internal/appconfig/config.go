package appconfig

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Version is injected from the release tag with go build -ldflags -X.
var Version = "dev"

type Config struct {
	Listen, DataDir, DBPath, WhatsAppDBPath, PublicDir      string
	InitialChatLimit, InitialMessagesPerChat, MaxMediaMB    int
	MediaDir                                                string
	MQTTEnabled                                             *bool
	MQTTBroker, MQTTUsername, MQTTPassword, MQTTTopicPrefix *string
	MirrorOnly                                              *bool
}

func env(name string) (string, bool) { v, ok := os.LookupEnv(name); return strings.TrimSpace(v), ok }
func envFirst(names ...string) (string, bool) {
	for _, n := range names {
		if v, ok := env(n); ok && v != "" {
			return v, true
		}
	}
	return "", false
}
func envInt(names []string, d int) int {
	if v, ok := envFirst(names...); ok {
		if n, e := strconv.Atoi(v); e == nil {
			return n
		}
	}
	return d
}
func envBool(name string) *bool {
	if v, ok := env(name); ok {
		if b, e := strconv.ParseBool(v); e == nil {
			return &b
		}
	}
	return nil
}
func envStringPtr(name string) *string {
	if v, ok := env(name); ok {
		return &v
	}
	return nil
}

type optString struct {
	set   bool
	value string
}

func (o *optString) String() string     { return o.value }
func (o *optString) Set(v string) error { o.set = true; o.value = v; return nil }

type optBool struct {
	set   bool
	value bool
}

func (o *optBool) String() string { return strconv.FormatBool(o.value) }
func (o *optBool) Set(v string) error {
	b, e := strconv.ParseBool(v)
	if e != nil {
		return e
	}
	o.set = true
	o.value = b
	return nil
}
func (o *optBool) IsBoolFlag() bool { return true }

func Parse(args []string) (Config, error) {
	data, _ := envFirst("WISP_DATA_DIR", "DATA_DIR")
	if data == "" {
		data = "./data"
	}
	listen, _ := envFirst("WISP_LISTEN")
	if listen == "" {
		if p, ok := envFirst("WISP_PORT", "PORT"); ok {
			listen = ":" + p
		} else {
			listen = ":8080"
		}
	}
	db, _ := envFirst("WISP_DB_PATH", "DB_PATH")
	if db == "" {
		db = data + "/wisp.db"
	}
	wadb, _ := envFirst("WISP_WHATSAPP_DB_PATH", "WHATSAPP_DB_PATH")
	if wadb == "" {
		wadb = data + "/whatsmeow.db"
	}
	media, _ := envFirst("WISP_MEDIA_DIR", "MEDIA_DIR")
	if media == "" {
		media = data + "/media"
	}
	public, _ := envFirst("WISP_PUBLIC_DIR", "PUBLIC_DIR")
	c := Config{Listen: listen, DataDir: data, DBPath: db, WhatsAppDBPath: wadb, MediaDir: media, PublicDir: public, InitialChatLimit: envInt([]string{"WISP_INITIAL_CHAT_LIMIT", "INITIAL_CHAT_LIMIT"}, 10), InitialMessagesPerChat: envInt([]string{"WISP_INITIAL_MESSAGES_PER_CHAT", "INITIAL_MESSAGES_PER_CHAT"}, 50), MaxMediaMB: envInt([]string{"WISP_MAX_MEDIA_MB", "MAX_MEDIA_MB"}, 100), MQTTEnabled: envBool("WISP_MQTT_ENABLED"), MQTTBroker: envStringPtr("WISP_MQTT_BROKER"), MQTTUsername: envStringPtr("WISP_MQTT_USERNAME"), MQTTPassword: envStringPtr("WISP_MQTT_PASSWORD"), MQTTTopicPrefix: envStringPtr("WISP_MQTT_TOPIC_PREFIX"), MirrorOnly: envBool("WISP_MIRROR_ONLY")}
	fs := flag.NewFlagSet("wisp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&c.Listen, "listen", c.Listen, "HTTP listen address")
	fs.StringVar(&c.DataDir, "data-dir", c.DataDir, "data directory")
	fs.StringVar(&c.DBPath, "db", c.DBPath, "Wisp SQLite database")
	fs.StringVar(&c.WhatsAppDBPath, "whatsapp-db", c.WhatsAppDBPath, "whatsmeow SQLite database")
	fs.StringVar(&c.MediaDir, "media-dir", c.MediaDir, "media directory")
	fs.StringVar(&c.PublicDir, "public-dir", c.PublicDir, "development web UI override")
	fs.IntVar(&c.InitialChatLimit, "initial-chat-limit", c.InitialChatLimit, "initial chats to keep")
	fs.IntVar(&c.InitialMessagesPerChat, "initial-messages-per-chat", c.InitialMessagesPerChat, "initial messages per chat")
	fs.IntVar(&c.MaxMediaMB, "max-media-mb", c.MaxMediaMB, "maximum downloaded media size in MB")
	var me optBool
	var mb, mu, mp, mt optString
	var mo optBool
	fs.Var(&me, "mqtt-enabled", "enable/disable MQTT")
	fs.Var(&mb, "mqtt-broker", "MQTT broker URL")
	fs.Var(&mu, "mqtt-username", "MQTT username")
	fs.Var(&mp, "mqtt-password", "MQTT password (prefer WISP_MQTT_PASSWORD)")
	fs.Var(&mt, "mqtt-topic-prefix", "MQTT topic prefix")
	fs.Var(&mo, "mirror-only", "disable sending messages")
	version := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if *version {
		return c, fmt.Errorf("__version__")
	}
	if me.set {
		c.MQTTEnabled = &me.value
	}
	if mb.set {
		c.MQTTBroker = &mb.value
	}
	if mu.set {
		c.MQTTUsername = &mu.value
	}
	if mp.set {
		c.MQTTPassword = &mp.value
	}
	if mt.set {
		c.MQTTTopicPrefix = &mt.value
	}
	if mo.set {
		c.MirrorOnly = &mo.value
	}
	if c.InitialChatLimit < 1 {
		return c, fmt.Errorf("initial-chat-limit must be at least 1")
	}
	if c.InitialMessagesPerChat < 1 {
		return c, fmt.Errorf("initial-messages-per-chat must be at least 1")
	}
	if c.MaxMediaMB < 1 {
		return c, fmt.Errorf("max-media-mb must be at least 1")
	}
	return c, nil
}
