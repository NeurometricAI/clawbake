package operator

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clawbakev1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
)

const finalizerName = "clawbake.io/finalizer"

// Notifier sends notifications when instance state changes.
type Notifier interface {
	NotifyInstanceReady(ctx context.Context, instanceName, userID string)
}

// +kubebuilder:rbac:groups=clawbake.io,resources=clawinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=clawbake.io,resources=clawinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=clawbake.io,resources=clawinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

type ClawInstanceReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	Recorder             record.EventRecorder
	Notifier             Notifier
	ServerNamespace      string
	DefaultGatewayConfig string
	TtydImage            string
	TtydPort             int32
	TtydCommand          string
	TtydResources        corev1.ResourceRequirements
}

func (r *ClawInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var instance clawbakev1alpha1.ClawInstance
	if err := r.Get(ctx, req.NamespacedName, &instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !instance.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&instance, finalizerName) {
			logger.Info("Deleting resources for ClawInstance", "userId", instance.Spec.UserId)
			instance.Status.Phase = clawbakev1alpha1.PhaseTerminating
			if err := r.Status().Update(ctx, &instance); err != nil {
				return ctrl.Result{}, err
			}

			if err := r.deleteUserNamespace(ctx, &instance); err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(&instance, finalizerName)
			if err := r.Update(ctx, &instance); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&instance, finalizerName) {
		controllerutil.AddFinalizer(&instance, finalizerName)
		if err := r.Update(ctx, &instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Set initial phase
	if instance.Status.Phase == "" {
		instance.Status.Phase = clawbakev1alpha1.PhasePending
		if err := r.Status().Update(ctx, &instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update phase to Creating
	if instance.Status.Phase == clawbakev1alpha1.PhasePending {
		instance.Status.Phase = clawbakev1alpha1.PhaseCreating
		if err := r.Status().Update(ctx, &instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile resources in order
	namespaceName := fmt.Sprintf("clawbake-%s", instance.Name)

	if err := r.reconcileNamespace(ctx, &instance, namespaceName); err != nil {
		return r.setFailed(ctx, &instance, "NamespaceFailed", err)
	}

	if err := r.reconcilePVC(ctx, &instance, namespaceName); err != nil {
		return r.setFailed(ctx, &instance, "PVCFailed", err)
	}

	if err := r.reconcileDeployment(ctx, &instance, namespaceName); err != nil {
		return r.setFailed(ctx, &instance, "DeploymentFailed", err)
	}

	if err := r.reconcileService(ctx, &instance, namespaceName); err != nil {
		return r.setFailed(ctx, &instance, "ServiceFailed", err)
	}

	if err := r.reconcileNetworkPolicy(ctx, &instance, namespaceName); err != nil {
		return r.setFailed(ctx, &instance, "NetworkPolicyFailed", err)
	}

	// Check deployment readiness before declaring Running
	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: "openclaw", Namespace: namespaceName}, deploy); err != nil {
		return ctrl.Result{}, err
	}

	instance.Status.Namespace = namespaceName

	if deploy.Status.ReadyReplicas >= 1 {
		wasRunning := instance.Status.Phase == clawbakev1alpha1.PhaseRunning
		instance.Status.Phase = clawbakev1alpha1.PhaseRunning
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "ReconcileComplete",
			Message:            "All resources reconciled successfully",
			ObservedGeneration: instance.Generation,
		})
		if err := r.Status().Update(ctx, &instance); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(&instance, corev1.EventTypeNormal, "Reconciled", "All resources reconciled successfully")
		logger.Info("Successfully reconciled ClawInstance", "userId", instance.Spec.UserId, "namespace", namespaceName)
		if !wasRunning && r.Notifier != nil {
			r.Notifier.NotifyInstanceReady(ctx, instance.Name, instance.Spec.UserId)
		}
		return ctrl.Result{}, nil
	}

	// Not ready yet — set Starting and requeue
	instance.Status.Phase = clawbakev1alpha1.PhaseStarting
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "WaitingForReady",
		Message:            "Waiting for deployment to become ready",
		ObservedGeneration: instance.Generation,
	})
	if err := r.Status().Update(ctx, &instance); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *ClawInstanceReconciler) setFailed(ctx context.Context, instance *clawbakev1alpha1.ClawInstance, reason string, err error) (ctrl.Result, error) {
	instance.Status.Phase = clawbakev1alpha1.PhaseFailed
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            err.Error(),
		ObservedGeneration: instance.Generation,
	})
	if statusErr := r.Status().Update(ctx, instance); statusErr != nil {
		return ctrl.Result{}, statusErr
	}
	r.Recorder.Event(instance, corev1.EventTypeWarning, reason, err.Error())
	return ctrl.Result{}, err
}

func (r *ClawInstanceReconciler) reconcileNamespace(ctx context.Context, instance *clawbakev1alpha1.ClawInstance, namespaceName string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ns, func() error {
		if ns.Labels == nil {
			ns.Labels = make(map[string]string)
		}
		ns.Labels["clawbake.io/instance"] = instance.Name
		ns.Labels["clawbake.io/user-id"] = instance.Spec.UserId
		return nil
	})
	return err
}

func (r *ClawInstanceReconciler) reconcilePVC(ctx context.Context, instance *clawbakev1alpha1.ClawInstance, namespaceName string) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openclaw-data",
			Namespace: namespaceName,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		if pvc.CreationTimestamp.IsZero() {
			pvc.Labels = map[string]string{
				"clawbake.io/instance": instance.Name,
			}
			size := instance.Spec.Storage.Size
			if size == "" {
				size = "5Gi"
			}
			pvc.Spec = corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(size),
					},
				},
			}
			if instance.Spec.Storage.StorageClass != "" {
				pvc.Spec.StorageClassName = &instance.Spec.Storage.StorageClass
			}
		}
		return nil
	})
	return err
}

func (r *ClawInstanceReconciler) reconcileDeployment(ctx context.Context, instance *clawbakev1alpha1.ClawInstance, namespaceName string) error {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openclaw",
			Namespace: namespaceName,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		labels := map[string]string{
			"app":                  "openclaw",
			"clawbake.io/instance": instance.Name,
		}
		replicas := int32(1)

		volumeMounts := []corev1.VolumeMount{
			{Name: "data", MountPath: "/data"},
			{Name: "data", MountPath: "/home/node/.openclaw", SubPath: "openclaw-config"},
		}
		volumes := []corev1.Volume{
			{
				Name: "data",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "openclaw-data",
					},
				},
			},
		}

		gatewayConfig := instance.Spec.GatewayConfig
		if gatewayConfig == "" {
			gatewayConfig = r.DefaultGatewayConfig
		}

		initContainers := []corev1.Container{
			{
				Name:  "write-config",
				Image: instance.Spec.Image,
				Command: []string{"sh", "-c",
					fmt.Sprintf(`test -f /home/node/.openclaw/openclaw.json || echo '%s' > /home/node/.openclaw/openclaw.json`, gatewayConfig),
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "data", MountPath: "/home/node/.openclaw", SubPath: "openclaw-config"},
				},
			},
		}

		containers := []corev1.Container{
			{
				Name:    "openclaw",
				Image:   instance.Spec.Image,
				Command: []string{"node", "/app/openclaw.mjs", "gateway", "run", "--bind", "lan", "--allow-unconfigured"},
				Ports: []corev1.ContainerPort{
					{
						Name:          "http",
						ContainerPort: 18789,
						Protocol:      corev1.ProtocolTCP,
					},
				},
				Env: []corev1.EnvVar{
					{Name: "NODE_OPTIONS", Value: "--max-old-space-size=1536 --disable-warning=ExperimentalWarning"},
					{Name: "OPENCLAW_NODE_OPTIONS_READY", Value: "1"},
					{Name: "OPENCLAW_GATEWAY_TOKEN", Value: instance.Spec.GatewayToken},
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/__openclaw/control-ui-config.json",
							Port: intstr.FromInt32(18789),
						},
					},
					InitialDelaySeconds: 5,
					PeriodSeconds:       5,
				},
				Resources:    buildResourceRequirements(instance.Spec.Resources),
				VolumeMounts: volumeMounts,
			},
		}

		if r.TtydCommand != "" {
			// Add emptyDir volume for the ttyd binary
			volumes = append(volumes, corev1.Volume{
				Name: "ttyd-bin",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})

			// Init container copies ttyd binary from the ttyd image
			initContainers = append(initContainers, corev1.Container{
				Name:    "install-ttyd",
				Image:   r.TtydImage,
				Command: []string{"cp", "/usr/bin/ttyd", "/ttyd-bin/ttyd"},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "ttyd-bin", MountPath: "/ttyd-bin"},
				},
			})

			ttydMounts := append(volumeMounts, corev1.VolumeMount{
				Name: "ttyd-bin", MountPath: "/ttyd-bin",
			})

			containers = append(containers, corev1.Container{
				Name:    "ttyd",
				Image:   instance.Spec.Image,
				Command: []string{"sh", "-c", r.TtydCommand},
				Env: []corev1.EnvVar{
					{Name: "OPENCLAW_GATEWAY_TOKEN", Value: instance.Spec.GatewayToken},
				},
				Ports: []corev1.ContainerPort{
					{
						Name:          "ttyd",
						ContainerPort: r.TtydPort,
						Protocol:      corev1.ProtocolTCP,
					},
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						TCPSocket: &corev1.TCPSocketAction{
							Port: intstr.FromInt32(r.TtydPort),
						},
					},
					InitialDelaySeconds: 10,
					PeriodSeconds:       5,
				},
				Resources:    r.TtydResources,
				VolumeMounts: ttydMounts,
			})
		}

		deploy.Labels = labels
		deploy.Spec = appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "openclaw"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup: ptr.To(int64(1000)),
					},
					EnableServiceLinks: ptr.To(false),
					InitContainers:     initContainers,
					Containers:         containers,
					Volumes:            volumes,
				},
			},
		}
		return nil
	})
	return err
}

func buildResourceRequirements(res clawbakev1alpha1.ClawInstanceResources) corev1.ResourceRequirements {
	reqs := corev1.ResourceRequirements{}
	if res.Requests.CPU != "" || res.Requests.Memory != "" {
		reqs.Requests = corev1.ResourceList{}
		if res.Requests.CPU != "" {
			reqs.Requests[corev1.ResourceCPU] = resource.MustParse(res.Requests.CPU)
		}
		if res.Requests.Memory != "" {
			reqs.Requests[corev1.ResourceMemory] = resource.MustParse(res.Requests.Memory)
		}
	}
	if res.Limits.CPU != "" || res.Limits.Memory != "" {
		reqs.Limits = corev1.ResourceList{}
		if res.Limits.CPU != "" {
			reqs.Limits[corev1.ResourceCPU] = resource.MustParse(res.Limits.CPU)
		}
		if res.Limits.Memory != "" {
			reqs.Limits[corev1.ResourceMemory] = resource.MustParse(res.Limits.Memory)
		}
	}
	return reqs
}

func (r *ClawInstanceReconciler) reconcileService(ctx context.Context, instance *clawbakev1alpha1.ClawInstance, namespaceName string) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openclaw",
			Namespace: namespaceName,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = map[string]string{
			"clawbake.io/instance": instance.Name,
		}
		ports := []corev1.ServicePort{
			{
				Name:       "http",
				Port:       18789,
				TargetPort: intstr.FromString("http"),
				Protocol:   corev1.ProtocolTCP,
			},
		}
		if r.TtydCommand != "" {
			ports = append(ports, corev1.ServicePort{
				Name:       "ttyd",
				Port:       r.TtydPort,
				TargetPort: intstr.FromString("ttyd"),
				Protocol:   corev1.ProtocolTCP,
			})
		}
		svc.Spec = corev1.ServiceSpec{
			Selector: map[string]string{"app": "openclaw"},
			Ports:    ports,
		}
		return nil
	})
	return err
}

func (r *ClawInstanceReconciler) reconcileNetworkPolicy(ctx context.Context, instance *clawbakev1alpha1.ClawInstance, namespaceName string) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-clawbake-server-only",
			Namespace: namespaceName,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, np, func() error {
		np.Labels = map[string]string{
			"clawbake.io/instance": instance.Name,
		}
		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{}, // selects all pods in namespace
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": r.ServerNamespace,
								},
							},
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/name":      "clawbake",
									"app.kubernetes.io/component": "server",
								},
							},
						},
					},
				},
			},
		}
		return nil
	})
	return err
}

func (r *ClawInstanceReconciler) deleteUserNamespace(ctx context.Context, instance *clawbakev1alpha1.ClawInstance) error {
	namespaceName := fmt.Sprintf("clawbake-%s", instance.Name)
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: namespaceName}, ns); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return r.Delete(ctx, ns)
}

func (r *ClawInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clawbakev1alpha1.ClawInstance{}).
		Named("clawinstance").
		Complete(r)
}
