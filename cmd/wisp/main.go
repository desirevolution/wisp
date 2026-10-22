package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"wisp/internal/appconfig"
	"wisp/internal/events"
	"wisp/internal/httpapi"
	"wisp/internal/mqttsvc"
	"wisp/internal/pushsvc"
	"wisp/internal/store"
	"wisp/internal/updates"
	"wisp/internal/webui"
	"wisp/internal/whatsapp"
)

func main() {
	cfg, err := appconfig.Parse(os.Args[1:])
	if err != nil {
		if err.Error() == "__version__" {
			fmt.Println("wisp", appconfig.Version)
			return
		}
		log.Fatal(err)
	}
	for _, p := range []string{cfg.DataDir, filepath.Dir(cfg.DBPath), filepath.Dir(cfg.WhatsAppDBPath), cfg.MediaDir} {
		if p != "." && p != "" {
			if err := os.MkdirAll(p, 0o755); err != nil {
				log.Fatal(err)
			}
		}
	}
	ctx, stop := shutdownContext(context.Background())
	defer stop()
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	// Mirror mode and MQTT are deployment policy, never mutable app settings.
	// Always override legacy DB values so an old UI-configured value cannot leak
	// into a new deployment. Defaults are send enabled and MQTT disabled.
	mirrorOnly := false
	if cfg.MirrorOnly != nil {
		mirrorOnly = *cfg.MirrorOnly
	}
	st.SetConfigOverride("mirrorOnly", strconv.FormatBool(mirrorOnly))
	st.SetConfigOverride("pushTestEnabled", strconv.FormatBool(cfg.PushTestEnabled))
	mqttEnabled := false
	if cfg.MQTTEnabled != nil {
		mqttEnabled = *cfg.MQTTEnabled
	}
	st.SetConfigOverride("mqttEnabled", strconv.FormatBool(mqttEnabled))
	mqttBroker, mqttUser, mqttPass, mqttTopic := "", "", "", "wisp"
	if cfg.MQTTBroker != nil {
		mqttBroker = *cfg.MQTTBroker
	}
	if cfg.MQTTUsername != nil {
		mqttUser = *cfg.MQTTUsername
	}
	if cfg.MQTTPassword != nil {
		mqttPass = *cfg.MQTTPassword
	}
	if cfg.MQTTTopicPrefix != nil {
		mqttTopic = *cfg.MQTTTopicPrefix
	}
	st.SetConfigOverride("mqttBroker", mqttBroker)
	st.SetConfigOverride("mqttUsername", mqttUser)
	st.SetConfigOverride("mqttPassword", mqttPass)
	st.SetConfigOverride("mqttTopicPrefix", mqttTopic)
	bus := events.New()
	updateChecker := updates.New(appconfig.Version, st, func(state updates.State) { bus.Publish(events.Event{Type: "update", Data: state}) })
	updateChecker.Start(ctx)
	mq := mqttsvc.New(st, bus)
	mq.Start()
	defer mq.Close()
	push, err := pushsvc.New(st, bus)
	if err != nil {
		log.Fatal(err)
	}
	push.Start()
	defer push.Close()
	wa := whatsapp.New(st, bus, whatsapp.Options{MediaDir: cfg.MediaDir, MaxMediaMB: cfg.MaxMediaMB, InitialChatLimit: cfg.InitialChatLimit, InitialMessagesPerChat: cfg.InitialMessagesPerChat})
	if err = wa.Start(ctx, cfg.WhatsAppDBPath); err != nil {
		log.Fatal(err)
	}
	defer wa.Close()
	api := httpapi.New(st, wa, bus, mq, push, webui.FS(), cfg.PublicDir)
	api.SetUpdateChecker(updateChecker)
	srv := &http.Server{Addr: cfg.Listen, Handler: api.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("Wisp Go %s listening on %s", appconfig.Version, srv.Addr)
		if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			log.Fatal(e)
		}
	}()
	<-ctx.Done()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(c)
}
