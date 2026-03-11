package k8s

import (
	"bytes"
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clawbakev1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
)

func NewScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(appsv1.AddToScheme(s))
	utilruntime.Must(clawbakev1alpha1.AddToScheme(s))
	return s
}

func NewClient() (client.Client, *rest.Config, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("getting kubeconfig: %w", err)
	}
	cl, err := client.New(cfg, client.Options{Scheme: NewScheme()})
	if err != nil {
		return nil, nil, fmt.Errorf("creating k8s client: %w", err)
	}
	return cl, cfg, nil
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

func UpdateInstance(ctx context.Context, cl client.Client, instance *clawbakev1alpha1.ClawInstance) error {
	return cl.Update(ctx, instance)
}

// ReadInstanceConfig reads openclaw.json from the running pod via exec.
// Returns the file contents and nil error on success.
// If the pod is not running or exec fails, returns an error.
func ReadInstanceConfig(ctx context.Context, restConfig *rest.Config, cl client.Client, instanceNamespace string) (string, error) {
	// Find the openclaw pod
	podList := &corev1.PodList{}
	err := cl.List(ctx, podList, client.InNamespace(instanceNamespace), client.MatchingLabels{"app": "openclaw"})
	if err != nil {
		return "", fmt.Errorf("listing pods: %w", err)
	}
	if len(podList.Items) == 0 {
		return "", fmt.Errorf("no openclaw pods found")
	}

	pod := podList.Items[0]
	if pod.Status.Phase != corev1.PodRunning {
		return "", fmt.Errorf("pod is not running (phase: %s)", pod.Status.Phase)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return "", fmt.Errorf("creating clientset: %w", err)
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(instanceNamespace).
		SubResource("exec").
		Param("container", "openclaw").
		Param("command", "cat").
		Param("command", "/home/node/.openclaw/openclaw.json").
		Param("stdout", "true").
		Param("stderr", "true")

	exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("creating executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", fmt.Errorf("exec failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}
