// Command talam-server runs the ingest API, LLM gateway, remediation broker,
// and management dashboard described in docs/components/server/README.md.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/nirvanagit/talam/internal/server"
	"github.com/nirvanagit/talam/internal/server/llm"
)

func main() {
	addr := flag.String("addr", ":8443", "listen address")
	storePath := flag.String("store", "talam-server-store.json", "path to the JSON history store (empty disables persistence)")
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig for ModelBinding/Secret lookups (defaults to in-cluster, falls back to $KUBECONFIG)")
	bindingNamespace := flag.String("model-binding-namespace", "talam-system", "namespace to look up the ModelBinding in")
	bindingName := flag.String("model-binding-name", "default", "name of the ModelBinding to watch")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	store, err := server.NewStore(*storePath)
	if err != nil {
		log.Error("failed to load store", "err", err)
		os.Exit(1)
	}

	fallback := defaultFallbackProvider(log)
	resolver := llm.NewBindingResolver(nil, nil, *bindingNamespace, *bindingName, fallback, log)

	if cfg, err := loadKubeConfig(*kubeconfig); err != nil {
		log.Warn("no Kubernetes config available; ModelBinding CRD disabled, using env/CLI provider only", "err", err)
	} else {
		dyn, derr := dynamic.NewForConfig(cfg)
		core, cerr := kubernetes.NewForConfig(cfg)
		if derr != nil || cerr != nil {
			log.Warn("failed to build Kubernetes clients; ModelBinding CRD disabled", "err", firstErr(derr, cerr))
		} else {
			resolver.Dynamic = dyn
			resolver.Core = core
			go resolver.Start(context.Background(), 30*time.Second)
			log.Info("watching ModelBinding for LLM provider selection", "namespace", *bindingNamespace, "name", *bindingName)
		}
	}

	gateway := &llm.Gateway{ProviderFunc: resolver.Current}
	orchestrator := &server.Orchestrator{Store: store, Gateway: gateway, Log: log}
	srv := &server.Server{Store: store, Orchestrator: orchestrator, Resolver: resolver, Log: log}

	httpServer := &http.Server{Addr: *addr, Handler: srv.Routes()}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("talam-server listening", "addr", *addr, "provider", resolver.Current().Name())
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}

// defaultFallbackProvider picks claude-cli when there's no API key but a
// local claude binary exists (laptop/dev convenience), otherwise Anthropic.
func defaultFallbackProvider(log *slog.Logger) llm.Provider {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return llm.NewAnthropic()
	}
	if _, err := exec.LookPath("claude"); err == nil {
		log.Info("no ANTHROPIC_API_KEY set; falling back to local claude CLI")
		return llm.NewClaudeCLI()
	}
	log.Warn("no ANTHROPIC_API_KEY and no claude CLI found; LLM calls will fail until a ModelBinding or credential is provided")
	return llm.NewAnthropic()
}

func loadKubeConfig(explicit string) (*rest.Config, error) {
	if explicit != "" {
		return clientcmd.BuildConfigFromFlags("", explicit)
	}
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		return clientcmd.BuildConfigFromFlags("", kc)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return clientcmd.BuildConfigFromFlags("", home+"/.kube/config")
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
