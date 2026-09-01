package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modbus-console/modbus-console/core/internal/api"
	"github.com/modbus-console/modbus-console/core/internal/engine"
	"github.com/modbus-console/modbus-console/core/internal/modbus"
)

var version = "dev"

func main() {
	apiAddress := flag.String("api-listen", envOr("MODBUS_CONSOLE_API_LISTEN", "127.0.0.1:17777"), "Core control API listen address")
	modbusAddress := flag.String("modbus-listen", envOr("MODBUS_CONSOLE_MODBUS_LISTEN", "127.0.0.1:1502"), "Modbus TCP slave listen address")
	strict := flag.Bool("strict-addressing", false, "return exception 02 for unmapped registers")
	flag.Parse()

	token, tokenSource, err := engine.LoadOrCreateToken(os.Getenv("MODBUS_CONSOLE_AUTH_TOKEN"))
	if err != nil {
		log.Fatalf("pairing token initialization failed: %v", err)
	}
	store := modbus.NewStore()
	store.Strict = *strict
	if err := modbus.SeedSWS(store); err != nil {
		log.Fatalf("SWS profile seed failed: %v", err)
	}
	slave := modbus.NewServer(store)
	actualModbus, err := slave.Start(*modbusAddress)
	if err != nil {
		log.Fatalf("Modbus TCP slave start failed: %v", err)
	}
	defer func() { _ = slave.Close() }()

	origins := strings.Split(envOr("MODBUS_CONSOLE_ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"), ",")
	rtuMaster := modbus.NewRTUMaster()
	rtuSlave := modbus.NewRTUSlave(store)
	defer func() { _ = rtuMaster.Close(); _ = rtuSlave.Close() }()
	apiServer := api.New(api.Config{Token: token, AllowedOrigins: origins, Version: version, ModbusAddress: actualModbus, Store: store, Slave: slave, RTUMaster: rtuMaster, RTUSlave: rtuSlave})
	httpServer := &http.Server{Addr: *apiAddress, Handler: apiServer.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}

	go func() {
		log.Printf("Modbus Console Core %s", version)
		log.Printf("Control API: http://%s", *apiAddress)
		log.Printf("Modbus TCP slave: %s", actualModbus)
		log.Printf("Pairing token source: %s (token is never logged)", tokenSource)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("control API failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	fmt.Println("Modbus Console Core stopped")
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
