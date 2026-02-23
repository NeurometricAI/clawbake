package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PhasePending     = "Pending"
	PhaseCreating    = "Creating"
	PhaseRunning     = "Running"
	PhaseFailed      = "Failed"
	PhaseTerminating = "Terminating"
)

// ClawInstanceSpec defines the desired state of a ClawInstance.
type ClawInstanceSpec struct {
	UserId       string               `json:"userId"`
	DisplayName  string               `json:"displayName"`
	Image        string               `json:"image"`
	GatewayToken string               `json:"gatewayToken,omitempty"`
	Resources    ClawInstanceResources `json:"resources,omitempty"`
	Storage      ClawInstanceStorage   `json:"storage,omitempty"`
	Ingress      ClawInstanceIngress   `json:"ingress,omitempty"`
}

type ClawInstanceResources struct {
	Requests ResourceList `json:"requests,omitempty"`
	Limits   ResourceList `json:"limits,omitempty"`
}

type ResourceList struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

type ClawInstanceStorage struct {
	Size         string `json:"size,omitempty"`
	StorageClass string `json:"storageClass,omitempty"`
}

type ClawInstanceIngress struct {
	Enabled bool   `json:"enabled,omitempty"`
	Host    string `json:"host,omitempty"`
}

// ClawInstanceStatus defines the observed state of a ClawInstance.
type ClawInstanceStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Namespace  string             `json:"namespace,omitempty"`
	URL        string             `json:"url,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.status.namespace`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ClawInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClawInstanceSpec   `json:"spec,omitempty"`
	Status ClawInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ClawInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClawInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClawInstance{}, &ClawInstanceList{})
}
