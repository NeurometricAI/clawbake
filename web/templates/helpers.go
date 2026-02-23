package templates

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func statusClass(phase string) string {
	switch phase {
	case "Running":
		return "running"
	case "Pending", "Creating":
		return "pending"
	case "Failed":
		return "failed"
	case "Terminating":
		return "terminating"
	default:
		return "unknown"
	}
}

func conditionStatus(s metav1.ConditionStatus) string {
	return string(s)
}
