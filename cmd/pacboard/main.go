// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command pacboard serves the pacboard dashboard for Tekton Pipelines-as-Code runs.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/M0Rf30/pacboard/internal/api"
	"github.com/M0Rf30/pacboard/internal/auth"
	"github.com/M0Rf30/pacboard/internal/github"
	"github.com/M0Rf30/pacboard/internal/kube"
	"github.com/M0Rf30/pacboard/internal/logs"
	"github.com/M0Rf30/pacboard/web"
)

// Set by -ldflags at build time.
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

const (
	envPrefix       = "PACBOARD_"
	shutdownTimeout = 15 * time.Second
)

type options struct {
	listen       string
	kubeconfig   string
	kubeContext  string
	namespaces   string
	authMode     string
	clientID     string
	clientSecret string
	githubURL    string
	allowedOrgs  string
	sessionKey   string
	publicURL    string
	devTokenEnv  string
	logLevel     string
	printVersion bool
}

func parseFlags(args []string) (options, error) {
	var o options

	fs := flag.NewFlagSet("pacboard", flag.ContinueOnError)
	fs.StringVar(&o.listen, "listen", ":8080", "address to listen on")
	fs.StringVar(
		&o.kubeconfig,
		"kubeconfig",
		"",
		"path to a kubeconfig (default: KUBECONFIG, ~/.kube/config, or the in-cluster service account)",
	)
	fs.StringVar(&o.kubeContext, "kube-context", "", "kubeconfig context to use")
	fs.StringVar(&o.namespaces, "namespaces", "", "comma-separated namespaces to watch (default: all)")
	fs.StringVar(
		&o.authMode,
		"auth-mode",
		auth.ModeGitHub,
		`"github" (OAuth login, organization gate) or "none" (read-only, development only)`,
	)
	fs.StringVar(&o.clientID, "github-client-id", "", "GitHub OAuth App / GitHub App client ID")
	fs.StringVar(
		&o.clientSecret,
		"github-client-secret",
		"",
		"GitHub client secret (prefer the PACBOARD_GITHUB_CLIENT_SECRET environment variable)",
	)
	fs.StringVar(&o.githubURL, "github-url", github.DefaultWebURL, "GitHub web URL; set it for GitHub Enterprise Server or Cloud")
	fs.StringVar(&o.allowedOrgs, "allowed-orgs", "", "comma-separated GitHub organizations whose members may use pacboard")
	fs.StringVar(&o.sessionKey, "session-key", "", "secret of at least 32 bytes encrypting session cookies (prefer PACBOARD_SESSION_KEY)")
	fs.StringVar(&o.publicURL, "public-url", "", "externally visible base URL, used for the OAuth callback <public-url>/auth/callback")
	fs.StringVar(&o.devTokenEnv, "dev-github-token-env", "",
		"development only: name of an environment variable holding a GitHub token; reruns are then posted as its user")
	fs.StringVar(&o.logLevel, "log-level", "info", "debug, info, warn or error")
	fs.BoolVar(&o.printVersion, "version", false, "print the version and exit")

	// Every flag also reads PACBOARD_<NAME>; the command line wins.
	given := map[string]bool{}

	if err := fs.Parse(args); err != nil {
		return o, fmt.Errorf("parse flags: %w", err)
	}

	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })

	var envErr error

	fs.VisitAll(func(f *flag.Flag) {
		if given[f.Name] || f.Name == "version" {
			return
		}

		key := envPrefix + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		if v, ok := os.LookupEnv(key); ok && v != "" {
			if err := f.Value.Set(v); err != nil {
				envErr = fmt.Errorf("%s: %w", key, err)
			}
		}
	})

	return o, envErr
}

func splitList(s string) []string {
	var out []string

	for item := range strings.SplitSeq(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}

	return out
}

func newLogger(level string) (*slog.Logger, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("invalid log level %q", level)
	}

	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})), nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pacboard:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	o, err := parseFlags(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}

	if err != nil {
		return err
	}

	if o.printVersion {
		fmt.Printf("pacboard %s (commit %s, built %s)\n", version, commit, buildTime)

		return nil
	}

	log, err := newLogger(o.logLevel)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	gh, err := github.New(o.githubURL)
	if err != nil {
		return fmt.Errorf("github url: %w", err)
	}

	authCfg := auth.Config{
		Mode: o.authMode, ClientID: o.clientID, ClientSecret: o.clientSecret, AllowedOrgs: splitList(o.allowedOrgs),
		SessionKey: o.sessionKey, PublicURL: o.publicURL, GitHub: gh, Logger: log,
	}

	if o.devTokenEnv != "" {
		authCfg.DevToken = os.Getenv(o.devTokenEnv)
		if authCfg.DevToken == "" {
			return fmt.Errorf("--dev-github-token-env: %s is empty", o.devTokenEnv)
		}
	}

	if authCfg.DevToken != "" && o.authMode != auth.ModeNone {
		return errors.New("--dev-github-token-env only works with --auth-mode none")
	}

	authSvc, err := auth.New(ctx, authCfg)
	if err != nil {
		return err
	}

	announce(log, o, authSvc)

	restCfg, err := kube.RestConfig(o.kubeconfig, o.kubeContext)
	if err != nil {
		return err
	}

	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("dynamic client: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("kubernetes client: %w", err)
	}

	store, err := kube.NewStore(dyn, splitList(o.namespaces), log)
	if err != nil {
		return err
	}

	store.Start(ctx)

	handler := api.New(api.Deps{
		Store:  store,
		Logs:   &logs.Streamer{Pods: logs.Kubernetes{Client: clientset}, TaskRuns: store},
		Auth:   authSvc,
		GitHub: gh,
		UI:     web.Handler(),
		Log:    log,
	})

	return serve(ctx, log, o.listen, handler)
}

// announce logs the effective security posture; a dev token is shouted about.
func announce(log *slog.Logger, o options, a *auth.Service) {
	log.Info(
		"starting pacboard",
		"version",
		version,
		"commit",
		commit,
		"authMode",
		o.authMode,
		"namespaces",
		o.namespaces,
		"listen",
		o.listen,
	)

	if o.authMode == auth.ModeNone {
		log.Warn("auth mode none: no login, the dashboard is open to anyone who can reach it; for local development only")
	}

	if login := a.DevLogin(); login != "" {
		log.Warn(
			"DEV GITHUB TOKEN IN USE: every visitor can post /test and /retest comments as this GitHub user; never use this in a deployment",
			"login",
			login,
			"env",
			o.devTokenEnv,
		)
	}
}

func serve(ctx context.Context, log *slog.Logger, addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No WriteTimeout: log and event streams are long-lived. Cancelling ctx on shutdown
		// ends them, so Shutdown does not wait for open streams.
		BaseContext: func(net.Listener) context.Context { return ctx },
		ErrorLog:    slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	errc := make(chan error, 1)

	go func() {
		log.Info("listening", "addr", addr)

		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}

		close(errc)
	}()

	select {
	case err := <-errc:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}

		return nil
	case <-ctx.Done():
	}

	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	return nil
}
