// Command talam-agent runs the scan loop and the CRD-native remediation
// pipeline described in docs/components/agent/README.md and ADR-0005. One
// agent per cluster.
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
	namespace := flag.String("namespace", "talam-system", "namespace to create/reconcile MeshIncident and MeshResolution objects in")
	scanInterval := flag.Duration("scan-interval", 30*time.Second, "how often to scan; docs default 5m, shorter locally for fast demo feedback")
	syncInterval := flag.Duration("sync-interval", 15*time.Second, "how often to sync MeshIncident/MeshResolution from talam-server (ADR-0005)")
	reconcileInterval := flag.Duration("reconcile-interval", 5*time.Second, "how often to reconcile triggered MeshResolutions and roll up MeshIncident completeness")
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
	httpClient := &http.Client{Timeout: 30 * time.Second}

	reporter := agent.NewReporter(*serverURL, *cluster, log)
	engine := &agent.Engine{
		Collector: collectistio.NewCollector(dyn, *cluster),
		Analyzers: analyzeristio.All(),
		Reporter:  reporter,
		Interval:  *scanInterval,
		Log:       log,
	}
	sync := &agent.CRDSync{
		ServerURL: *serverURL,
		Cluster:   *cluster,
		Namespace: *namespace,
		Dynamic:   dyn,
		Client:    httpClient,
		Log:       log,
	}
	resolutions := &agent.ResolutionReconciler{
		Namespace: *namespace,
		Dynamic:   dyn,
		Applier: &agent.Applier{
			ServerURL: *serverURL,
			Cluster:   *cluster,
			Dynamic:   dyn,
			Client:    httpClient,
			Log:       log,
		},
		Log: log,
	}
	incidents := &agent.IncidentReconciler{Namespace: *namespace, Dynamic: dyn, Log: log}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("talam-agent starting", "cluster", *cluster, "server", *serverURL, "namespace", *namespace, "scanInterval", *scanInterval)
	go sync.Run(ctx, *syncInterval)
	go resolutions.Run(ctx, *reconcileInterval)
	go incidents.Run(ctx, *reconcileInterval)
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
