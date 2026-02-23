//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clawbakev1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
)

func setupClient(t *testing.T) client.Client {
	t.Helper()

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = clientcmd.RecommendedHomeFile
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("failed to build kubeconfig: %v", err)
	}

	if err := clawbakev1alpha1.AddToScheme(scheme.Scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	cl, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	return cl
}

func TestCRDIsInstalled(t *testing.T) {
	cl := setupClient(t)
	ctx := context.Background()

	list := &clawbakev1alpha1.ClawInstanceList{}
	if err := cl.List(ctx, list, client.InNamespace("clawbake")); err != nil {
		t.Fatalf("CRD not installed or accessible: %v", err)
	}
	t.Logf("Found %d ClawInstances in clawbake namespace", len(list.Items))
}

func TestCreateClawInstance(t *testing.T) {
	cl := setupClient(t)
	ctx := context.Background()

	// Ensure clawbake namespace exists
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "clawbake"},
	}
	if err := cl.Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("failed to create namespace: %v", err)
	}

	name := fmt.Sprintf("test-user-%d", time.Now().Unix())
	instance := &clawbakev1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "clawbake",
		},
		Spec: clawbakev1alpha1.ClawInstanceSpec{
			UserId:      name,
			DisplayName: "Integration Test User",
			Image:       "nginx:alpine",
			Resources: clawbakev1alpha1.ClawInstanceResources{
				Requests: clawbakev1alpha1.ResourceList{CPU: "50m", Memory: "64Mi"},
				Limits:   clawbakev1alpha1.ResourceList{CPU: "100m", Memory: "128Mi"},
			},
			Storage: clawbakev1alpha1.ClawInstanceStorage{Size: "1Gi"},
			Ingress: clawbakev1alpha1.ClawInstanceIngress{
				Enabled: true,
				Host:    fmt.Sprintf("%s.claw.test", name),
			},
		},
	}

	// Create the CR
	if err := cl.Create(ctx, instance); err != nil {
		t.Fatalf("failed to create ClawInstance: %v", err)
	}

	t.Cleanup(func() {
		// Clean up: delete CR and user namespace
		_ = cl.Delete(context.Background(), instance)
		userNs := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("clawbake-%s", name)}}
		_ = cl.Delete(context.Background(), userNs)
	})

	// Verify CR was created
	got := &clawbakev1alpha1.ClawInstance{}
	if err := cl.Get(ctx, types.NamespacedName{Name: name, Namespace: "clawbake"}, got); err != nil {
		t.Fatalf("failed to get created ClawInstance: %v", err)
	}

	if got.Spec.UserId != name {
		t.Errorf("expected userId %s, got %s", name, got.Spec.UserId)
	}
	if got.Spec.Image != "nginx:alpine" {
		t.Errorf("expected image nginx:alpine, got %s", got.Spec.Image)
	}
}

func TestOperatorReconciliation(t *testing.T) {
	if os.Getenv("OPERATOR_RUNNING") != "true" {
		t.Skip("Skipping: OPERATOR_RUNNING not set (operator must be running in cluster)")
	}

	cl := setupClient(t)
	ctx := context.Background()

	name := fmt.Sprintf("test-reconcile-%d", time.Now().Unix())
	userNs := fmt.Sprintf("clawbake-%s", name)

	instance := &clawbakev1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "clawbake",
		},
		Spec: clawbakev1alpha1.ClawInstanceSpec{
			UserId:      name,
			DisplayName: "Reconcile Test",
			Image:       "nginx:alpine",
			Resources: clawbakev1alpha1.ClawInstanceResources{
				Requests: clawbakev1alpha1.ResourceList{CPU: "50m", Memory: "64Mi"},
				Limits:   clawbakev1alpha1.ResourceList{CPU: "100m", Memory: "128Mi"},
			},
			Storage: clawbakev1alpha1.ClawInstanceStorage{Size: "1Gi"},
			Ingress: clawbakev1alpha1.ClawInstanceIngress{Enabled: true, Host: fmt.Sprintf("%s.claw.test", name)},
		},
	}

	if err := cl.Create(ctx, instance); err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}

	t.Cleanup(func() {
		_ = cl.Delete(context.Background(), instance)
		time.Sleep(5 * time.Second)
	})

	// Wait for namespace to be created by operator
	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		ns := &corev1.Namespace{}
		if err := cl.Get(ctx, types.NamespacedName{Name: userNs}, ns); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("timed out waiting for namespace %s: %v", userNs, err)
	}

	// Verify deployment
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		deploy := &appsv1.Deployment{}
		if err := cl.Get(ctx, types.NamespacedName{Name: "openclaw", Namespace: userNs}, deploy); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("timed out waiting for deployment: %v", err)
	}

	// Verify service
	svc := &corev1.Service{}
	if err := cl.Get(ctx, types.NamespacedName{Name: "openclaw", Namespace: userNs}, svc); err != nil {
		t.Fatalf("expected service to exist: %v", err)
	}

	// Verify ingress
	ing := &networkingv1.Ingress{}
	if err := cl.Get(ctx, types.NamespacedName{Name: "openclaw", Namespace: userNs}, ing); err != nil {
		t.Fatalf("expected ingress to exist: %v", err)
	}
	if ing.Spec.Rules[0].Host != fmt.Sprintf("%s.claw.test", name) {
		t.Errorf("unexpected ingress host: %s", ing.Spec.Rules[0].Host)
	}

	// Verify PVC
	pvc := &corev1.PersistentVolumeClaim{}
	if err := cl.Get(ctx, types.NamespacedName{Name: "openclaw-data", Namespace: userNs}, pvc); err != nil {
		t.Fatalf("expected PVC to exist: %v", err)
	}

	// Verify status updated
	updated := &clawbakev1alpha1.ClawInstance{}
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		if err := cl.Get(ctx, types.NamespacedName{Name: name, Namespace: "clawbake"}, updated); err != nil {
			return false, err
		}
		return updated.Status.Phase == clawbakev1alpha1.PhaseRunning, nil
	})
	if err != nil {
		t.Fatalf("timed out waiting for Running phase: %v (current: %s)", err, updated.Status.Phase)
	}
}
