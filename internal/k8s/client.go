package k8s

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clawbakev1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
)

func NewScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(appsv1.AddToScheme(s))
	utilruntime.Must(networkingv1.AddToScheme(s))
	utilruntime.Must(clawbakev1alpha1.AddToScheme(s))
	return s
}

func NewClient() (client.Client, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("getting kubeconfig: %w", err)
	}
	cl, err := client.New(cfg, client.Options{Scheme: NewScheme()})
	if err != nil {
		return nil, fmt.Errorf("creating k8s client: %w", err)
	}
	return cl, nil
}

func CreateInstance(ctx context.Context, cl client.Client, instance *clawbakev1alpha1.ClawInstance) error {
	return cl.Create(ctx, instance)
}

func GetInstance(ctx context.Context, cl client.Client, namespace, name string) (*clawbakev1alpha1.ClawInstance, error) {
	instance := &clawbakev1alpha1.ClawInstance{}
	err := cl.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, instance)
	if err != nil {
		return nil, err
	}
	return instance, nil
}

func ListInstances(ctx context.Context, cl client.Client, namespace string) ([]clawbakev1alpha1.ClawInstance, error) {
	list := &clawbakev1alpha1.ClawInstanceList{}
	err := cl.List(ctx, list, client.InNamespace(namespace))
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func DeleteInstance(ctx context.Context, cl client.Client, namespace, name string) error {
	instance, err := GetInstance(ctx, cl, namespace, name)
	if err != nil {
		return err
	}
	return cl.Delete(ctx, instance)
}
