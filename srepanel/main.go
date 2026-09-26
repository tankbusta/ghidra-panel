package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"go.mkw.re/ghidra-panel/ghidra"
	"golang.org/x/sync/errgroup"

	"go.mkw.re/ghidra-panel/database"
	"go.mkw.re/ghidra-panel/discord"
	"go.mkw.re/ghidra-panel/oidc"
	"go.mkw.re/ghidra-panel/token"
	"go.mkw.re/ghidra-panel/web"
)

func main() {
	// cli args
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "rename":
			os.Args = os.Args[1:]
			dbPath := flag.String("db", defaultDatabase, "SQLite database file path or postgres:// URL (supports $env:NAME)")
			argUserID := flag.Uint64("user-id", 0, "ID of user to rename")
			argUser := flag.String("user", "", "new username")
			flag.Parse()

			db, err := openDatabase(*dbPath)
			if err != nil {
				log.Fatal(err)
			}
			defer db.Close()

			if err := db.SetUsername(context.Background(), *argUserID, *argUser); err != nil {
				log.Fatal(err)
			}
			return
		case "set-password":
			os.Args = os.Args[1:]
			dbPath := flag.String("db", defaultDatabase, "SQLite database file path or postgres:// URL (supports $env:NAME)")
			argUserID := flag.Uint64("user-id", 0, "user id to set password for")
			argUser := flag.String("user", "", "user to set password for")
			argPass := flag.String("pass", "", "password to set")
			flag.Parse()
			updateAccount(*dbPath, *argUserID, *argUser, *argPass)
			return
		}
	}

	// prod args
	configPath := flag.String("config", "ghidra_panel.json", "path to config file")
	secretsPath := flag.String("secrets", "ghidra_panel.secrets.json", "path to secrets file")
	dbFlag := flag.String("db", "", "SQLite database file path or postgres:// URL (supports $env:NAME), overrides config \"database\" (default \""+defaultDatabase+"\")")
	listen := flag.String("listen", ":8080", "listen address")
	cmdInit := flag.Bool("init", false, "initialize database and exit")
	dev := flag.Bool("dev", false, "enable development mode")

	flag.Parse()

	// Read config

	configJSON, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	var cfg config
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		log.Fatal(err)
	}
	if err := cfg.resolveEnv(); err != nil {
		log.Fatal(err)
	}
	cfg.OIDC.setDefaults()
	if !*cmdInit {
		cfg.validate()
	}

	// Read secrets

	if _, err := os.Stat(*secretsPath); os.IsNotExist(err) {
		generateSecrets(*secretsPath)
	}
	secrets, err := ReadSecrets(*secretsPath)
	if err != nil {
		log.Fatal(err)
	}

	// Open database

	dsn, err := databaseDSN(*dbFlag, cfg.Database)
	if err != nil {
		log.Fatal("database: ", err)
	}
	db, err := database.Open(dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Setup app context

	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	group, ctx := errgroup.WithContext(ctx)

	// Setup gRPC client

	grpcAddr := cfg.Ghidra.GRPCAddr
	if grpcAddr == "" {
		grpcAddr = ghidra.DefaultGrpcAddr
	}
	client, err := ghidra.Connect(grpcAddr)
	if err != nil {
		log.Fatal(err)
	}

	// Setup web server

	if *cmdInit {
		return
	}

	// The Discord application provides the webhook name and avatar
	var app *discord.Application
	if cfg.Discord.BotToken != "" {
		app, err = discord.GetApplication(ctx, cfg.Discord.BotToken)
		if err != nil {
			log.Fatal(err)
		}
	}

	var auth *discord.Auth
	if cfg.discordLoginEnabled() {
		auth = discord.NewAuth(cfg.Discord.ClientID, cfg.Discord.ClientSecret, cfg.BaseURL+"/redirect")
	}

	var oidcAuth *oidc.Auth
	if cfg.OIDC.enabled() {
		oidcAuth, err = oidc.NewAuth(
			ctx,
			cfg.OIDC.Issuer,
			cfg.OIDC.ClientID,
			cfg.OIDC.ClientSecret,
			cfg.BaseURL+"/oidc/redirect",
			cfg.OIDC.Scopes,
			cfg.OIDC.UsernameClaim,
		)
		if err != nil {
			log.Fatal(err)
		}
		log.Println("OIDC login enabled for issuer", oidcAuth.Issuer)
	}

	issuer := token.NewIssuer(secrets.HMACSecret)

	webConfig := web.Config{
		BaseURL:           cfg.BaseURL,
		GhidraEndpoint:    &cfg.Ghidra.Endpoint,
		Links:             cfg.Links,
		DiscordApp:        app,
		DiscordWebhookURL: cfg.Discord.WebhookURL,
		Dev:               *dev,
		SuperAdmins:       cfg.SuperAdmins,
		OIDCDisplayName:   cfg.OIDC.DisplayName,
		OIDCSuperAdmins:   cfg.OIDC.SuperAdmins,
	}
	server, err := web.NewServer(&webConfig, db, auth, oidcAuth, &issuer, client)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	log.Println("Server listening on", *listen)

	httpServer := http.Server{
		Addr:    *listen,
		Handler: mux,
	}
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	group.Go(func() error {
		<-ctx.Done()
		return httpServer.Shutdown(ctx)
	})

	if err := group.Wait(); err != nil {
		log.Println(err)
	}
	log.Println("Server stopped gracefully")
}

// openDatabase opens the database given by a subcommand's -db flag.
func openDatabase(dbFlag string) (*database.DB, error) {
	dsn, err := databaseDSN(dbFlag, "")
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	return database.Open(dsn)
}

func updateAccount(dbPath string, userID uint64, user, pass string) {
	db, err := openDatabase(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	err = db.UpdateAccount(ctx, userID, user, pass)
	if errors.Is(err, database.ErrUserNotFound) {
		err = db.CreateAccount(ctx, userID, user, pass)
	}
	if err != nil {
		log.Fatal(err)
	}
}
