package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/nirvanagit/talam/pkg/api"
	"github.com/nirvanagit/talam/pkg/mesh"
)

// Reporter pushes finding batches to talam-server. If the server is
// unreachable the batch is buffered and retried on the next report — the
// scan loop never blocks on server availability (ADR-0001).
type Reporter struct {
	ServerURL string
	Cluster   string
	Client    *http.Client
	Log       *slog.Logger

	pending []mesh.Finding
}

func NewReporter(serverURL, cluster string, log *slog.Logger) *Reporter {
	return &Reporter{
		ServerURL: serverURL,
		Cluster:   cluster,
		Client:    &http.Client{Timeout: 15 * time.Second},
		Log:       log,
	}
}

// Report sends findings (plus any buffered backlog). An empty batch is still
// sent: it tells the server "this cluster scanned clean", which is what lets
// incidents resolve.
func (r *Reporter) Report(ctx context.Context, findings []mesh.Finding) {
	batch := append(r.pending, findings...)
	if err := r.post(ctx, batch); err != nil {
		r.Log.Warn("report failed, buffering", "buffered", len(batch), "err", err)
		const maxBuffer = 10000
		if len(batch) > maxBuffer {
			batch = batch[len(batch)-maxBuffer:]
		}
		r.pending = batch
		return
	}
	r.pending = nil
}

func (r *Reporter) post(ctx context.Context, findings []mesh.Finding) error {
	body, err := json.Marshal(api.ReportRequest{Cluster: r.Cluster, Findings: findings})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.ServerURL+"/v1/findings", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %s", resp.Status)
	}
	return nil
}
