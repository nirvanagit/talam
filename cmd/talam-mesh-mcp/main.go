// Command talam-mesh-mcp is the read-only Istio-introspection MCP server
// talam-server's evidence-enrichment step calls (ADR-0006). One per
// mesh-bearing cluster, deployed next to the agent — see
// docs/decisions/0006-mcp-evidence-enrichment.md for why it runs in-cluster
// rather than being owned by talam-server itself.
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nirvanagit/talam/internal/meshmcp"
)

func main() {
	addr := flag.String("addr", ":9090", "listen address")
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig (defaults to in-cluster, falls back to $KUBECONFIG)")
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

	server := meshmcp.New(dyn)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)

	log.Info("talam-mesh-mcp listening", "addr", *addr)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Error("http server failed", "err", err)
		os.Exit(1)
	}
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
