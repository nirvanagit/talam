// Command talam-agent runs the scan loop and remediation applier described
// in docs/components/agent/README.md. One agent per cluster.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/nirvanagit/talam/internal/agent"
	analyzeristio "github.com/nirvanagit/talam/internal/analyzers/istio"
	collectistio "github.com/nirvanagit/talam/internal/collect/istio"
)

func main() {
	cluster := flag.String("cluster", "", "this cluster's name, as reported to talam-server (required)")
	serverURL := flag.String("server", "", "talam-server base URL, e.g. http://talam-server:8443 (required)")
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig (defaults to in-cluster, falls back to $KUBECONFIG)")
	scanInterval := flag.Duration("scan-interval", 30*time.Second, "how often to scan; docs default 5m, shorter locally for fast demo feedback")
	applyInterval := flag.Duration("apply-interval", 15*time.Second, "how often to poll for approved remediation proposals")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if *cluster == "" || *serverURL == "" {
		log.Error("both -cluster and -server are required")
		os.Exit(2)
	}

	cfg, err := loadKubeConfig(*kubeconfig)
	if err != nil {
		log.Error("failed to load kubeconfig", "err", err)
		os.Exit(1)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		log.Error("failed to build dynamic client", "err", err)
		os.Exit(1)
	}

	reporter := agent.NewReporter(*serverURL, *cluster, log)
	engine := &agent.Engine{
		Collector: collectistio.NewCollector(dyn, *cluster),
		Analyzers: analyzeristio.All(),
		Reporter:  reporter,
		Interval:  *scanInterval,
		Log:       log,
	}
	applier := &agent.Applier{
		ServerURL: *serverURL,
		Cluster:   *cluster,
		Dynamic:   dyn,
		Client:    &http.Client{Timeout: 30 * time.Second},
		Log:       log,
		Interval:  *applyInterval,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("talam-agent starting", "cluster", *cluster, "server", *serverURL, "scanInterval", *scanInterval)
	go applier.Run(ctx)
	engine.Run(ctx)
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
