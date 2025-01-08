package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// Book is a specification for a Book resource
type Book struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BookSpec   `json:"spec"`
	Status BookStatus `json:"status,omitempty"`
}

// BookSpec is the spec for a Book resource
type BookSpec struct {
	DeploymentName string           `json:"deploymentName"`
	Replicas       *int32           `json:"replicas"`
	Container      corev1.Container `json:"container"`
}

// BookStatus is the status for a Book resource
type BookStatus struct {
	AvailableReplicas int32 `json:"availableReplicas"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BookList is a list of Book resources
type BookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Book `json:"items"`
}
