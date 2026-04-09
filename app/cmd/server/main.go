package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/cbrgm/githubevents/githubevents"
	"github.com/google/go-github/v69/github"

	"github.com/co2-lab/polvo/internal/agent"
	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/gitclient"
	"github.com/co2-lab/polvo/internal/guide"
	"github.com/co2-lab/polvo/internal/pipeline"
	"github.com/co2-lab/polvo/internal/provider"
	"github.com/co2-lab/polvo/internal/webhook"
)

func main() {
	logger := slog.Default()

	configPath := envOr("POLVO_CONFIG", "polvo.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("loading config", "error", err)
		os.Exit(1)
	}

	// Setup providers
	registry, err := provider.NewRegistry(cfg.Providers)
	if err != nil {
		logger.Error("creating providers", "error", err)
		os.Exit(1)
	}

	// Setup git client
	var gitClient gitclient.GitPlatform
	token := os.Getenv("GITHUB_TOKEN")
	appIDStr := os.Getenv("GITHUB_APP_ID")
	installIDStr := os.Getenv("GITHUB_INSTALLATION_ID")
	privateKeyPath := os.Getenv("GITHUB_PRIVATE_KEY_PATH")

	if appIDStr != "" && installIDStr != "" && privateKeyPath != "" {
		appID, _ := strconv.ParseInt(appIDStr, 10, 64)
		installID, _ := strconv.ParseInt(installIDStr, 10, 64)
		gitClient, err = gitclient.NewGitHubClient(appID, installID, privateKeyPath)
		if err != nil {
			logger.Error("creating github app client", "error", err)
			os.Exit(1)
		}
	} else if token != "" {
		gitClient = gitclient.NewGitHubClientWithToken(token)
	} else {
		logger.Error("no GitHub credentials configured (set GITHUB_TOKEN or GITHUB_APP_ID + GITHUB_INSTALLATION_ID + GITHUB_PRIVATE_KEY_PATH)")
		os.Exit(1)
	}

	// Setup pipeline
	resolver := guide.NewResolver(".", cfg)
	executor := agent.NewExecutor(resolver, registry, cfg)
	scheduler := pipeline.NewScheduler(executor, gitClient, cfg, logger, nil)

	// Setup webhook handlers
	pushHandler := webhook.NewPushHandler(scheduler, cfg, logger)
	prHandler := webhook.NewPRHandler(logger)

	// Setup GitHub events
	webhookSecret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	handle := githubevents.New(webhookSecret)
	handle.OnPushEventAny(func(_ string, _ string, event *github.PushEvent) error {
		pushHandler.Handle(event)
		return nil
	})
	handle.OnPullRequestEventAny(func(_ string, _ string, event *github.PullRequestEvent) error {
		prHandler.Handle(event)
		return nil
	})

	// Start server
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		if err := handle.HandleEventRequest(r); err != nil {
			logger.Error("handling webhook", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	port := envOr("PORT", "8080")
	addr := ":" + port
	logger.Info("starting polvo server", "addr", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
