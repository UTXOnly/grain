package main

import (
	"fmt"
	"grain/app/src/api"
	"grain/app/src/routes"
	"grain/config"
	configTypes "grain/config/types"
	relay "grain/server"
	"grain/server/db/mongo"
	"grain/server/utils"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/websocket"
)

func main() {
    utils.EnsureFileExists("config.yml", "app/static/examples/config.example.yml")
    utils.EnsureFileExists("whitelist.yml", "app/static/examples/whitelist.example.yml")
    utils.EnsureFileExists("blacklist.yml", "app/static/examples/blacklist.example.yml")
    utils.EnsureFileExists("relay_metadata.json", "app/static/examples/relay_metadata.example.json")

    restartChan := make(chan struct{})
    go config.WatchConfigFile("config.yml", restartChan)
    go config.WatchConfigFile("whitelist.yml", restartChan)
    go config.WatchConfigFile("blacklist.yml", restartChan)
    go config.WatchConfigFile("relay_metadata.json", restartChan)

    signalChan := make(chan os.Signal, 1)
    signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

    var wg sync.WaitGroup
    for {
        wg.Add(1)

        cfg, err := config.LoadConfig("config.yml")
        if err != nil {
            log.Fatal("Error loading config: ", err)
        }

        _, err = config.LoadWhitelistConfig("whitelist.yml")
        if err != nil {
            log.Fatal("Error loading whitelist config: ", err)
        }

        _, err = config.LoadBlacklistConfig("blacklist.yml")
        if err != nil {
            log.Fatal("Error loading blacklist config: ", err)
        }

        // Start event purging in the background.
        go mongo.ScheduleEventPurging(cfg)

        config.SetResourceLimit(&cfg.ResourceLimits)
        client, err := mongo.InitDB(cfg)
        if err != nil {
            log.Fatal("Error initializing database: ", err)
        }

        config.SetRateLimit(cfg)
        config.SetSizeLimit(cfg)
        config.ClearTemporaryBans()

        err = utils.LoadRelayMetadataJSON()
        if err != nil {
            log.Fatal("Failed to load relay metadata: ", err)
        }

        mux := setupRoutes()
        server := startServer(cfg, mux, &wg)

        // Monitor for server restart or shutdown signals.
        select {
        case <-restartChan:
            log.Println("Restarting server...")
            server.Close()
            wg.Wait()
            time.Sleep(3 * time.Second)
        case <-signalChan:
            log.Println("Shutting down server...")
            server.Close()
            mongo.DisconnectDB(client)
            wg.Wait()
            return
        }
    }
}

func setupRoutes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", ListenAndServe)
	mux.HandleFunc("/import-results", api.ImportEvents)
	mux.HandleFunc("/import-events", routes.ImportEvents)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("app/static"))))
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "app/static/img/favicon.ico")
	})
	return mux
}

func startServer(config *configTypes.ServerConfig, mux *http.ServeMux, wg *sync.WaitGroup) *http.Server {
	server := &http.Server{
		Addr:         config.Server.Port,
		Handler:      mux,
		ReadTimeout:  time.Duration(config.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(config.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(config.Server.IdleTimeout) * time.Second,
	}

	go func() {
		defer wg.Done() // Notify that the server goroutine is done
		fmt.Printf("Server is running on http://localhost%s\n", config.Server.Port)
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			fmt.Println("Error starting server:", err)
		}
	}()

	return server
}

var wsServer = &websocket.Server{
	Handshake: func(config *websocket.Config, r *http.Request) error {
		// Skip origin check
		return nil
	},
	Handler: websocket.Handler(relay.WebSocketHandler),
}

func ListenAndServe(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Upgrade") == "websocket" {
		wsServer.ServeHTTP(w, r)
	} else if r.Header.Get("Accept") == "application/nostr+json" {
		utils.RelayInfoHandler(w, r)
	} else {
		routes.IndexHandler(w, r)
	}
}
