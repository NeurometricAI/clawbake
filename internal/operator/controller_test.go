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

type mockNotifier struct {
	calls []mockNotifyCall
}

type mockNotifyCall struct {
	InstanceName string
	UserID       string
}

func (m *mockNotifier) NotifyInstanceReady(_ context.Context, instanceName, userID string) {
	m.calls = append(m.calls, mockNotifyCall{InstanceName: instanceName, UserID: userID})
}

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
		Client:      k8sClient,
		Scheme:      k8sClient.Scheme(),
		Recorder:    record.NewFakeRecorder(10),
		TtydImage:   "tsl0922/ttyd:alpine",
		TtydPort:    7681,
		TtydCommand: "/ttyd-bin/ttyd -p 7681 node /app/openclaw.mjs tui",
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

	// Verify deployment was created with 2 containers (openclaw + ttyd)
	deploy := &appsv1.Deployment{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "openclaw", Namespace: "clawbake-test-instance"}, deploy); err != nil {
		t.Fatalf("expected deployment to exist: %v", err)
	}
	if deploy.Spec.Template.Spec.Containers[0].Image != "ghcr.io/openclaw/openclaw:latest" {
		t.Errorf("unexpected image: %s", deploy.Spec.Template.Spec.Containers[0].Image)
	}
	if len(deploy.Spec.Template.Spec.Containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(deploy.Spec.Template.Spec.Containers))
	}
	if deploy.Spec.Template.Spec.Containers[1].Name != "ttyd" {
		t.Errorf("expected second container name 'ttyd', got %q", deploy.Spec.Template.Spec.Containers[1].Name)
	}

	// Verify service was created with 2 ports
	svc := &corev1.Service{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "openclaw", Namespace: "clawbake-test-instance"}, svc); err != nil {
		t.Fatalf("expected service to exist: %v", err)
	}
	if len(svc.Spec.Ports) != 2 {
		t.Fatalf("expected 2 service ports, got %d", len(svc.Spec.Ports))
	}

	// Verify PVC was created
	pvc := &corev1.PersistentVolumeClaim{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "openclaw-data", Namespace: "clawbake-test-instance"}, pvc); err != nil {
		t.Fatalf("expected PVC to exist: %v", err)
	}

	// Verify status was updated (no real kubelet in envtest, so deployment has 0 ready replicas)
	updated := &clawbakev1alpha1.ClawInstance{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, updated); err != nil {
		t.Fatalf("failed to get updated instance: %v", err)
	}
	if updated.Status.Phase != clawbakev1alpha1.PhaseStarting {
		t.Errorf("expected phase Starting, got %s", updated.Status.Phase)
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

	// After full reconcile, should be Starting (no ready replicas in envtest)
	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if result.RequeueAfter == 0 {
		t.Error("expected requeue when deployment is not ready")
	}

	if err := k8sClient.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, updated); err != nil {
		t.Fatalf("failed to get instance: %v", err)
	}
	if updated.Status.Phase != clawbakev1alpha1.PhaseStarting {
		t.Errorf("expected phase Starting, got %s", updated.Status.Phase)
	}

	// Check Ready condition is False with WaitingForReady reason
	found := false
	for _, c := range updated.Status.Conditions {
		if c.Type == "Ready" {
			found = true
			if c.Status != metav1.ConditionFalse {
				t.Errorf("expected Ready condition to be False, got %s", c.Status)
			}
			if c.Reason != "WaitingForReady" {
				t.Errorf("expected reason WaitingForReady, got %s", c.Reason)
			}
		}
	}
	if !found {
		t.Error("expected Ready condition to exist")
	}
}

func TestReconcileStartingToRunning(t *testing.T) {
	k8sClient, _ := setupEnvtest(t)
	ctx := context.Background()

	instance := newTestInstance()
	if err := k8sClient.Create(ctx, instance); err != nil {
		t.Fatalf("failed to create ClawInstance: %v", err)
	}

	notifier := &mockNotifier{}
	reconciler := &ClawInstanceReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
		Notifier: notifier,
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

	// Second reconcile creates resources, sets Starting
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	// Simulate deployment becoming ready by updating its status
	deploy := &appsv1.Deployment{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "openclaw", Namespace: "clawbake-test-instance"}, deploy); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}
	deploy.Status.ReadyReplicas = 1
	deploy.Status.Replicas = 1
	if err := k8sClient.Status().Update(ctx, deploy); err != nil {
		t.Fatalf("failed to update deployment status: %v", err)
	}

	// Reconcile again — should transition to Running
	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Error("expected no requeue when deployment is ready")
	}

	updated := &clawbakev1alpha1.ClawInstance{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, updated); err != nil {
		t.Fatalf("failed to get updated instance: %v", err)
	}
	if updated.Status.Phase != clawbakev1alpha1.PhaseRunning {
		t.Errorf("expected phase Running, got %s", updated.Status.Phase)
	}

	// Check Ready condition is True
	found := false
	for _, c := range updated.Status.Conditions {
		if c.Type == "Ready" {
			found = true
			if c.Status != metav1.ConditionTrue {
				t.Errorf("expected Ready condition to be True, got %s", c.Status)
			}
			if c.Reason != "ReconcileComplete" {
				t.Errorf("expected reason ReconcileComplete, got %s", c.Reason)
			}
		}
	}
	if !found {
		t.Error("expected Ready condition to exist")
	}

	// Verify notifier was called exactly once with correct args
	if len(notifier.calls) != 1 {
		t.Fatalf("expected 1 notification call, got %d", len(notifier.calls))
	}
	if notifier.calls[0].InstanceName != "test-instance" {
		t.Errorf("expected instanceName 'test-instance', got %q", notifier.calls[0].InstanceName)
	}
	if notifier.calls[0].UserID != "testuser" {
		t.Errorf("expected userID 'testuser', got %q", notifier.calls[0].UserID)
	}
}

func TestReconcileRunningNoDuplicateNotification(t *testing.T) {
	k8sClient, _ := setupEnvtest(t)
	ctx := context.Background()

	instance := newTestInstance()
	instance.Name = "test-running-dup"
	if err := k8sClient.Create(ctx, instance); err != nil {
		t.Fatalf("failed to create ClawInstance: %v", err)
	}

	notifier := &mockNotifier{}
	reconciler := &ClawInstanceReconciler{
		Client:   k8sClient,
		Scheme:   k8sClient.Scheme(),
		Recorder: record.NewFakeRecorder(10),
		Notifier: notifier,
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

	// Simulate deployment becoming ready
	deploy := &appsv1.Deployment{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "openclaw", Namespace: "clawbake-test-running-dup"}, deploy); err != nil {
		t.Fatalf("failed to get deployment: %v", err)
	}
	deploy.Status.ReadyReplicas = 1
	deploy.Status.Replicas = 1
	if err := k8sClient.Status().Update(ctx, deploy); err != nil {
		t.Fatalf("failed to update deployment status: %v", err)
	}

	// First reconcile transitions to Running — should notify
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("expected 1 notification after first Running reconcile, got %d", len(notifier.calls))
	}

	// Second reconcile — already Running, should NOT notify again
	if _, err := reconciler.Reconcile(ctx, req); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Errorf("expected still 1 notification after re-reconcile, got %d", len(notifier.calls))
	}
}

func TestReconcileCreateWithoutTtyd(t *testing.T) {
	k8sClient, _ := setupEnvtest(t)
	ctx := context.Background()

	instance := newTestInstance()
	instance.Name = "test-no-ttyd"
	if err := k8sClient.Create(ctx, instance); err != nil {
		t.Fatalf("failed to create ClawInstance: %v", err)
	}

	reconciler := &ClawInstanceReconciler{
		Client:      k8sClient,
		Scheme:      k8sClient.Scheme(),
		Recorder:    record.NewFakeRecorder(10),
		TtydCommand: "", // no ttyd
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

	// Verify deployment has only 1 container (no ttyd)
	deploy := &appsv1.Deployment{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "openclaw", Namespace: "clawbake-test-no-ttyd"}, deploy); err != nil {
		t.Fatalf("expected deployment to exist: %v", err)
	}
	if len(deploy.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(deploy.Spec.Template.Spec.Containers))
	}

	// Verify service has only 1 port
	svc := &corev1.Service{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: "openclaw", Namespace: "clawbake-test-no-ttyd"}, svc); err != nil {
		t.Fatalf("expected service to exist: %v", err)
	}
	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("expected 1 service port, got %d", len(svc.Spec.Ports))
	}
}

// Ensure ctrl import is used
var _ = ctrl.Log
var _ = time.Second
