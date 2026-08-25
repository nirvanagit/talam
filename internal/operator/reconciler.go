// Package operator implements the talam-operator reconciliation loop
// described in docs/components/operator/README.md. It is deliberately dumb:
// it reconciles MeshDiagnostics into an agent Deployment/RBAC and watches
// agent health. It imports nothing from internal/agent, internal/server, or
// internal/server/llm — there is no code path here to mesh data or an LLM
// provider, which is the property ADR-0001 relies on, not just a convention.
package operator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/nirvanagit/talam/api/v1alpha1"
)

var gvrMeshDiagnostics = schema.GroupVersionResource{
	Group: v1alpha1.GroupName, Version: "v1alpha1", Resource: "meshdiagnostics",
}

// Reconciler polls MeshDiagnostics objects and drives cluster state to match
// them. Poll-based rather than an informer/controller-runtime manager to keep
// the operator's dependency footprint (and its blast radius) small — see the
// package doc.
type Reconciler struct {
	Dynamic   dynamic.Interface
	Core      kubernetes.Interface
	Namespace string // namespace the agent Deployment/RBAC objects are created in
	Log       *slog.Logger
}

func (r *Reconciler) Run(ctx context.Context, interval time.Duration) {
	r.reconcileAll(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reconcileAll(ctx)
		}
	}
}

func (r *Reconciler) reconcileAll(ctx context.Context) {
	list, err := r.Dynamic.Resource(gvrMeshDiagnostics).List(ctx, metav1.ListOptions{})
	if err != nil {
		r.Log.Error("listing MeshDiagnostics failed", "err", err)
		return
	}
	for i := range list.Items {
		if err := r.reconcileOne(ctx, &list.Items[i]); err != nil {
			r.Log.Error("reconcile failed", "meshdiagnostics", list.Items[i].GetName(), "err", err)
		}
	}
}

func (r *Reconciler) reconcileOne(ctx context.Context, md *unstructured.Unstructured) error {
	name := md.GetName()
	log := r.Log.With("meshdiagnostics", name)

	serverEndpoint, _, _ := unstructured.NestedString(md.Object, "spec", "serverEndpoint")
	if serverEndpoint == "" {
		return fmt.Errorf("spec.serverEndpoint is required")
	}
	agentImage, _, _ := unstructured.NestedString(md.Object, "spec", "agentImage")
	if agentImage == "" {
		agentImage = "talam-agent:local"
	}
	clusterName, _, _ := unstructured.NestedString(md.Object, "spec", "clusterName")
	if clusterName == "" {
		clusterName = name
	}

	if err := r.ensureRBAC(ctx, name); err != nil {
		return fmt.Errorf("rbac: %w", err)
	}
	dep, err := r.ensureDeployment(ctx, name, clusterName, serverEndpoint, agentImage)
	if err != nil {
		return fmt.Errorf("deployment: %w", err)
	}

	healthy := dep.Status.ReadyReplicas > 0 && dep.Status.ReadyReplicas == *dep.Spec.Replicas
	if err := r.updateStatus(ctx, md, healthy); err != nil {
		log.Warn("status update failed", "err", err)
	}
	return nil
}

// resourceNames derives object names from the MeshDiagnostics name so
// multiple MeshDiagnostics objects (unusual, but not forbidden) don't collide.
func resourceNames(mdName string) (sa, role, binding, deployment string) {
	base := "talam-agent-" + mdName
	return base, base, base, base
}

func (r *Reconciler) ensureRBAC(ctx context.Context, mdName string) error {
	saName, roleName, bindingName, _ := resourceNames(mdName)

	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: saName, Namespace: r.Namespace}}
	if _, err := r.Core.CoreV1().ServiceAccounts(r.Namespace).Create(ctx, sa, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	// Read-only on core + Istio API groups; write verbs only on the
	// patch-application path, scoped to Istio CRDs — never Secret, never RBAC
	// objects (docs/concepts/security-model.md).
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: roleName, Labels: map[string]string{managedByLabel: managedByValue}},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"services", "pods", "endpoints"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"networking.istio.io"}, Resources: []string{"*"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"security.istio.io"}, Resources: []string{"*"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{"networking.istio.io"}, Resources: []string{"destinationrules", "virtualservices", "gateways", "serviceentries"}, Verbs: []string{"patch"}},
		},
	}
	if err := r.applyClusterRole(ctx, role); err != nil {
		return err
	}

	// Create-only, deliberately: this operator never updates an existing
	// ClusterRoleBinding (see deploy/operator/rbac.yaml, which grants no
	// "update" verb on clusterrolebindings — only what this code path uses).
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: bindingName, Labels: map[string]string{managedByLabel: managedByValue}},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: roleName},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: saName, Namespace: r.Namespace}},
	}
	if _, err := r.Core.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	// MeshIncident/MeshResolution (ADR-0005) are namespaced and only ever
	// touched by the agent in its own namespace, so — unlike the Istio
	// access above, which is genuinely cluster-wide — this is a namespaced
	// Role, not a ClusterRole.
	crdRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: roleName, Namespace: r.Namespace, Labels: map[string]string{managedByLabel: managedByValue}},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"talam.dev"}, Resources: []string{"meshincidents", "meshresolutions"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch"}},
			{APIGroups: []string{"talam.dev"}, Resources: []string{"meshincidents/status", "meshresolutions/status"}, Verbs: []string{"get", "update", "patch"}},
		},
	}
	if err := r.applyRole(ctx, crdRole); err != nil {
		return err
	}
	crdBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: bindingName, Namespace: r.Namespace, Labels: map[string]string{managedByLabel: managedByValue}},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: roleName},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: saName, Namespace: r.Namespace}},
	}
	if _, err := r.Core.RbacV1().RoleBindings(r.Namespace).Create(ctx, crdBinding, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (r *Reconciler) applyRole(ctx context.Context, role *rbacv1.Role) error {
	existing, err := r.Core.RbacV1().Roles(role.Namespace).Get(ctx, role.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err := r.Core.RbacV1().Roles(role.Namespace).Create(ctx, role, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if existing.Labels[managedByLabel] != managedByValue {
		return fmt.Errorf("Role %q in %q exists but isn't labeled %s=%s — refusing to overwrite an object this operator didn't create", role.Name, role.Namespace, managedByLabel, managedByValue)
	}
	existing.Rules = role.Rules
	_, err = r.Core.RbacV1().Roles(role.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

const (
	managedByLabel = "talam.dev/managed-by"
	managedByValue = "talam-operator"
)

// applyClusterRole creates or updates the given ClusterRole. It refuses to
// update an existing object that doesn't carry this operator's managed-by
// label — application-level defense in depth against the operator's own
// broad update permission on the ClusterRole resource type (RBAC can't scope
// "update" to objects it created, since child object names are derived from
// arbitrary MeshDiagnostics names; see deploy/operator/rbac.yaml) ever
// clobbering an unrelated ClusterRole that happens to collide on name.
func (r *Reconciler) applyClusterRole(ctx context.Context, role *rbacv1.ClusterRole) error {
	existing, err := r.Core.RbacV1().ClusterRoles().Get(ctx, role.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err := r.Core.RbacV1().ClusterRoles().Create(ctx, role, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if existing.Labels[managedByLabel] != managedByValue {
		return fmt.Errorf("ClusterRole %q exists but isn't labeled %s=%s — refusing to overwrite an object this operator didn't create", role.Name, managedByLabel, managedByValue)
	}
	existing.Rules = role.Rules
	_, err = r.Core.RbacV1().ClusterRoles().Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (r *Reconciler) ensureDeployment(ctx context.Context, mdName, clusterName, serverEndpoint, image string) (*appsv1.Deployment, error) {
	saName, _, _, depName := resourceNames(mdName)
	replicas := int32(1)
	labels := map[string]string{"app": depName, "talam.dev/component": "agent"}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: depName, Namespace: r.Namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: saName,
					Containers: []corev1.Container{{
						Name:  "talam-agent",
						Image: image,
						Args: []string{
							"-cluster=" + clusterName,
							"-server=" + serverEndpoint,
						},
					}},
				},
			},
		},
	}

	existing, err := r.Core.AppsV1().Deployments(r.Namespace).Get(ctx, depName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return r.Core.AppsV1().Deployments(r.Namespace).Create(ctx, dep, metav1.CreateOptions{})
	}
	if err != nil {
		return nil, err
	}
	existing.Spec.Replicas = &replicas
	existing.Spec.Template = dep.Spec.Template
	return r.Core.AppsV1().Deployments(r.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
}

func (r *Reconciler) updateStatus(ctx context.Context, md *unstructured.Unstructured, healthy bool) error {
	fresh, err := r.Dynamic.Resource(gvrMeshDiagnostics).Get(ctx, md.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}
	if err := unstructured.SetNestedField(fresh.Object, healthy, "status", "agentHealthy"); err != nil {
		return err
	}
	_, err = r.Dynamic.Resource(gvrMeshDiagnostics).UpdateStatus(ctx, fresh, metav1.UpdateOptions{})
	return err
}
