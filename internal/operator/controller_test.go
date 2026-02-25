package operator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clawbakev1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
)

func setupEnvtest(t *testing.T) (client.Client, *envtest.Environment) {
	t.Helper()

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("failed to start envtest: %v", err)
	}

	err = clawbakev1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	t.Cleanup(func() {
		if err := testEnv.Stop(); err != nil {
			t.Errorf("failed to stop envtest: %v", err)
		}
	})

	return k8sClient, testEnv
}

func newTestInstance() *clawbakev1alpha1.ClawInstance {
	return &clawbakev1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "default",
		},
		Spec: clawbakev1alpha1.ClawInstanceSpec{
			UserId: "testuser",
			Image:  "ghcr.io/openclaw/openclaw:latest",
			Resources: clawbakev1alpha1.ClawInstanceResources{
				Requests: clawbakev1alpha1.ResourceList{CPU: "100m", Memory: "256Mi"},
				Limits:   clawbakev1alpha1.ResourceList{CPU: "500m", Memory: "512Mi"},
			},
			Storage: clawbakev1alpha1.ClawInstanceStorage{Size: "5Gi"},
		},
	}
}

func TestReconcileCreate(t *testing.T) {
	k8sClient, _ := setupEnvtest(t)
	ctx := context.Background()

	instance := newTestInstance()
	if err := k8sClient.Create(ctx, instance); err != nil {
		t.Fatalf("failed to create ClawInstance: %v", err)
	}

	reconciler := &ClawInstanceReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}

	// First reconcile adds finalizer
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	// Second reconcile creates resources
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	// Verify namespace was created
	ns := &corev1.Namespace{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "clawbake-test-instance"}, ns); err != nil {
		t.Fatalf("expected namespace clawbake-test-instance to exist: %v", err)
	}
	if ns.Labels["clawbake.io/user-id"] != "testuser" {
		t.Errorf("expected user-id label 'testuser', got '%s'", ns.Labels["clawbake.io/user-id"])
	}

	// Verify deployment was created
	deploy := &appsv1.Deployment{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "openclaw", Namespace: "clawbake-test-instance"}, deploy); err != nil {
		t.Fatalf("expected deployment to exist: %v", err)
	}
	if deploy.Spec.Template.Spec.Containers[0].Image != "ghcr.io/openclaw/openclaw:latest" {
		t.Errorf("unexpected image: %s", deploy.Spec.Template.Spec.Containers[0].Image)
	}

	// Verify service was created
	svc := &corev1.Service{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "openclaw", Namespace: "clawbake-test-instance"}, svc); err != nil {
		t.Fatalf("expected service to exist: %v", err)
	}

	// Verify PVC was created
	pvc := &corev1.PersistentVolumeClaim{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "openclaw-data", Namespace: "clawbake-test-instance"}, pvc); err != nil {
		t.Fatalf("expected PVC to exist: %v", err)
	}

	// Verify status was updated
	updated := &clawbakev1alpha1.ClawInstance{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, updated); err != nil {
		t.Fatalf("failed to get updated instance: %v", err)
	}
	if updated.Status.Phase != clawbakev1alpha1.PhaseRunning {
		t.Errorf("expected phase Running, got %s", updated.Status.Phase)
	}
	if updated.Status.Namespace != "clawbake-test-instance" {
		t.Errorf("expected namespace clawbake-test-instance, got %s", updated.Status.Namespace)
	}
}

func TestReconcileDelete(t *testing.T) {
	k8sClient, _ := setupEnvtest(t)
	ctx := context.Background()

	instance := newTestInstance()
	if err := k8sClient.Create(ctx, instance); err != nil {
		t.Fatalf("failed to create ClawInstance: %v", err)
	}

	reconciler := &ClawInstanceReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}

	// Reconcile to create resources (finalizer + resources)
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	// Verify namespace exists
	ns := &corev1.Namespace{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "clawbake-test-instance"}, ns); err != nil {
		t.Fatalf("expected namespace to exist before deletion: %v", err)
	}

	// Delete the instance
	if err := k8sClient.Delete(ctx, instance); err != nil {
		t.Fatalf("failed to delete ClawInstance: %v", err)
	}

	// Reconcile deletion
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("delete reconcile failed: %v", err)
	}

	// Verify the finalizer was removed (instance should be gone or finalizer removed)
	updated := &clawbakev1alpha1.ClawInstance{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, updated)
	if err == nil {
		// If still exists, finalizer should be removed
		for _, f := range updated.Finalizers {
			if f == finalizerName {
				t.Error("expected finalizer to be removed after delete reconcile")
			}
		}
	}
}

func TestReconcileStatusUpdates(t *testing.T) {
	k8sClient, _ := setupEnvtest(t)
	ctx := context.Background()

	instance := newTestInstance()
	if err := k8sClient.Create(ctx, instance); err != nil {
		t.Fatalf("failed to create ClawInstance: %v", err)
	}

	reconciler := &ClawInstanceReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}

	// First reconcile - adds finalizer, sets Pending
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	updated := &clawbakev1alpha1.ClawInstance{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, updated); err != nil {
		t.Fatalf("failed to get instance: %v", err)
	}

	// After full reconcile, should be Running
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	if err := k8sClient.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, updated); err != nil {
		t.Fatalf("failed to get instance: %v", err)
	}
	if updated.Status.Phase != clawbakev1alpha1.PhaseRunning {
		t.Errorf("expected phase Running, got %s", updated.Status.Phase)
	}

	// Check Ready condition
	found := false
	for _, c := range updated.Status.Conditions {
		if c.Type == "Ready" {
			found = true
			if c.Status != metav1.ConditionTrue {
				t.Errorf("expected Ready condition to be True, got %s", c.Status)
			}
		}
	}
	if !found {
		t.Error("expected Ready condition to exist")
	}
}

// Ensure ctrl import is used
var _ = ctrl.Log
var _ = time.Second
