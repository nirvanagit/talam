// Package agent implements the talam-agent runtime: a scan loop that builds
// snapshots, runs the registered analyzer set, and ships findings to
// talam-server; plus the applier that executes human-approved remediation.
package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/nirvanagit/talam/pkg/mesh"
)

// Snapshotter is implemented by backend collectors.
type Snapshotter interface {
	Snapshot(ctx context.Context) (*mesh.MeshSnapshot, error)
}

// Engine runs the analyzer set on a fixed interval. v0.1 runs every analyzer
// each tick; honoring Trigger() == OnChange via watches is roadmap.
type Engine struct {
	Collector Snapshotter
	Analyzers []mesh.Analyzer
	Reporter  *Reporter
	Interval  time.Duration
	Log       *slog.Logger
}

// Run blocks until ctx is cancelled, scanning immediately and then every
// Interval. Scan errors are logged, never fatal — the agent must keep
// scanning through transient API-server trouble.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.Interval)
	defer ticker.Stop()
	e.scanOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.scanOnce(ctx)
		}
	}
}

func (e *Engine) scanOnce(ctx context.Context) {
	snap, err := e.Collector.Snapshot(ctx)
	if err != nil {
		e.Log.Error("snapshot failed", "err", err)
		return
	}
	var all []mesh.Finding
	for _, a := range e.Analyzers {
		if a.Backend() != snap.Backend {
			continue
		}
		findings, err := a.Analyze(ctx, snap)
		if err != nil {
			e.Log.Error("analyzer failed", "analyzer", a.ID(), "err", err)
			continue
		}
		all = append(all, findings...)
	}
	e.Log.Info("scan complete",
		"services", len(snap.Core.Services), "pods", len(snap.Core.Pods), "findings", len(all))
	e.Reporter.Report(ctx, all)
}
