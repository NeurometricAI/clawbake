package k8s_test

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
	"github.com/clawbake/clawbake/internal/k8s"
)

func TestCreateAndGetInstance(t *testing.T) {
	scheme := k8s.NewScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	ctx := context.Background()
	instance := &v1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "default",
		},
		Spec: v1alpha1.ClawInstanceSpec{
			UserId: "user-1",
			Image:  "openclaw:latest",
		},
	}

	if err := k8s.CreateInstance(ctx, cl, instance); err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}

	got, err := k8s.GetInstance(ctx, cl, "default", "test-instance")
	if err != nil {
		t.Fatalf("failed to get instance: %v", err)
	}
	if got.Spec.UserId != "user-1" {
		t.Errorf("expected UserId 'user-1', got %q", got.Spec.UserId)
	}
}

func TestListInstances(t *testing.T) {
	scheme := k8s.NewScheme()

	existing := &v1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "inst-1",
			Namespace: "ns",
		},
		Spec: v1alpha1.ClawInstanceSpec{
			UserId: "user-1",
			Image:  "openclaw:latest",
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	items, err := k8s.ListInstances(context.Background(), cl, "ns")
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestDeleteInstance(t *testing.T) {
	scheme := k8s.NewScheme()

	existing := &v1alpha1.ClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "to-delete",
			Namespace: "ns",
		},
		Spec: v1alpha1.ClawInstanceSpec{
			UserId: "user-1",
			Image:  "openclaw:latest",
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	if err := k8s.DeleteInstance(context.Background(), cl, "ns", "to-delete"); err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	_, err := k8s.GetInstance(context.Background(), cl, "ns", "to-delete")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestGetInstanceNotFound(t *testing.T) {
	scheme := k8s.NewScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	_, err := k8s.GetInstance(context.Background(), cl, "ns", "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent instance")
	}
}
