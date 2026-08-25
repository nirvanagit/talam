// Package istio collects the cluster state the Istio analyzer set needs and
// projects it into a mesh.MeshSnapshot. Collection uses the dynamic client
// against both core and networking.istio.io API groups, so talam carries no
// dependency on istio/client-go.
//
// Per the security model, only config shape and metadata are collected —
// no Secrets, no request payloads.
package istio

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/nirvanagit/talam/internal/istiostate"
	"github.com/nirvanagit/talam/pkg/mesh"
)

var (
	gvrServices         = schema.GroupVersionResource{Version: "v1", Resource: "services"}
	gvrPods             = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	gvrDestinationRules = schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "destinationrules"}
	gvrVirtualServices  = schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "virtualservices"}
	gvrGateways         = schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "gateways"}
	gvrServiceEntries   = schema.GroupVersionResource{Group: "networking.istio.io", Version: "v1", Resource: "serviceentries"}
)

// Collector builds MeshSnapshots from a live cluster.
type Collector struct {
	client  dynamic.Interface
	cluster string
}

func NewCollector(client dynamic.Interface, clusterName string) *Collector {
	return &Collector{client: client, cluster: clusterName}
}

// Snapshot lists all relevant resources and projects them into a snapshot.
func (c *Collector) Snapshot(ctx context.Context) (*mesh.MeshSnapshot, error) {
	snap := &mesh.MeshSnapshot{
		Backend:     mesh.BackendIstio,
		Cluster:     c.cluster,
		CollectedAt: time.Now().UTC(),
	}
	st := &istiostate.State{}
	snap.BackendState = st

	if err := c.each(ctx, gvrServices, func(u *unstructured.Unstructured) {
		selector, _, _ := unstructured.NestedStringMap(u.Object, "spec", "selector")
		snap.Core.Services = append(snap.Core.Services, mesh.Service{
			Namespace: u.GetNamespace(), Name: u.GetName(), Selector: selector,
		})
	}); err != nil {
		return nil, err
	}

	if err := c.each(ctx, gvrPods, func(u *unstructured.Unstructured) {
		snap.Core.Pods = append(snap.Core.Pods, mesh.Pod{
			Namespace: u.GetNamespace(), Name: u.GetName(),
			Labels: u.GetLabels(), Ready: podReady(u),
		})
	}); err != nil {
		return nil, err
	}

	if err := c.each(ctx, gvrDestinationRules, func(u *unstructured.Unstructured) {
		host, _, _ := unstructured.NestedString(u.Object, "spec", "host")
		dr := istiostate.DestinationRule{Namespace: u.GetNamespace(), Name: u.GetName(), Host: host, ResourceVersion: u.GetResourceVersion()}
		subsets, _, _ := unstructured.NestedSlice(u.Object, "spec", "subsets")
		for _, s := range subsets {
			sm, ok := s.(map[string]any)
			if !ok {
				continue
			}
			name, _, _ := unstructured.NestedString(sm, "name")
			labels, _, _ := unstructured.NestedStringMap(sm, "labels")
			dr.Subsets = append(dr.Subsets, istiostate.Subset{Name: name, Labels: labels})
		}
		st.DestinationRules = append(st.DestinationRules, dr)
	}); err != nil {
		return nil, err
	}

	if err := c.each(ctx, gvrVirtualServices, func(u *unstructured.Unstructured) {
		vs := istiostate.VirtualService{Namespace: u.GetNamespace(), Name: u.GetName(), ResourceVersion: u.GetResourceVersion()}
		vs.Hosts, _, _ = unstructured.NestedStringSlice(u.Object, "spec", "hosts")
		vs.Gateways, _, _ = unstructured.NestedStringSlice(u.Object, "spec", "gateways")
		for _, proto := range []string{"http", "tcp", "tls"} {
			routes, _, _ := unstructured.NestedSlice(u.Object, "spec", proto)
			for _, r := range routes {
				rm, ok := r.(map[string]any)
				if !ok {
					continue
				}
				dests, _, _ := unstructured.NestedSlice(rm, "route")
				for _, d := range dests {
					dm, ok := d.(map[string]any)
					if !ok {
						continue
					}
					if host, _, _ := unstructured.NestedString(dm, "destination", "host"); host != "" {
						vs.DestinationHosts = append(vs.DestinationHosts, host)
					}
				}
			}
		}
		st.VirtualServices = append(st.VirtualServices, vs)
	}); err != nil {
		return nil, err
	}

	if err := c.each(ctx, gvrGateways, func(u *unstructured.Unstructured) {
		selector, _, _ := unstructured.NestedStringMap(u.Object, "spec", "selector")
		st.Gateways = append(st.Gateways, istiostate.Gateway{
			Namespace: u.GetNamespace(), Name: u.GetName(), Selector: selector, ResourceVersion: u.GetResourceVersion(),
		})
	}); err != nil {
		return nil, err
	}

	if err := c.each(ctx, gvrServiceEntries, func(u *unstructured.Unstructured) {
		hosts, _, _ := unstructured.NestedStringSlice(u.Object, "spec", "hosts")
		st.ServiceEntries = append(st.ServiceEntries, istiostate.ServiceEntry{
			Namespace: u.GetNamespace(), Name: u.GetName(), Hosts: hosts, ResourceVersion: u.GetResourceVersion(),
		})
	}); err != nil {
		return nil, err
	}

	return snap, nil
}

func (c *Collector) each(ctx context.Context, gvr schema.GroupVersionResource, fn func(*unstructured.Unstructured)) error {
	list, err := c.client.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing %s: %w", gvr.Resource, err)
	}
	for i := range list.Items {
		fn(&list.Items[i])
	}
	return nil
}

func podReady(u *unstructured.Unstructured) bool {
	phase, _, _ := unstructured.NestedString(u.Object, "status", "phase")
	if phase != "Running" {
		return false
	}
	conditions, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	for _, c := range conditions {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		typ, _, _ := unstructured.NestedString(cm, "type")
		status, _, _ := unstructured.NestedString(cm, "status")
		if typ == "Ready" {
			return status == "True"
		}
	}
	return false
}
