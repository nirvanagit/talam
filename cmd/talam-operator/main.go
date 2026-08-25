// Command talam-operator reconciles MeshDiagnostics into an agent Deployment
// and RBAC, per docs/components/operator/README.md.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/nirvanagit/talam/internal/operator"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig (defaults to in-cluster, falls back to $KUBECONFIG)")
	namespace := flag.String("namespace", "talam-system", "namespace to create agent Deployments/RBAC in")
	interval := flag.Duration("reconcile-interval", 15*time.Second, "how often to reconcile MeshDiagnostics")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

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
	core, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Error("failed to build core client", "err", err)
		os.Exit(1)
	}

	rec := &operator.Reconciler{Dynamic: dyn, Core: core, Namespace: *namespace, Log: log}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("talam-operator starting", "namespace", *namespace, "reconcileInterval", *interval)
	rec.Run(ctx, *interval)
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
