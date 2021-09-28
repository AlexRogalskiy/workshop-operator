package kubernetes

import (
	workshopv1 "github.com/stakater/workshop-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NewNamespace creates a new namespace/project
func NewNamespace(workshop *workshopv1.Workshop, scheme *runtime.Scheme, name string) *corev1.Namespace {

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	// Set Workshop instance as the owner and controller
	/**
	Error: cluster-scoped resource must not have a namespace-scoped owner
	err := ctrl.SetControllerReference(workshop, namespace, scheme)
	if err != nil {
		log.Error(err, " - Failed to set SetControllerReference for Namespace - %s", name)
	}
	**/
	return namespace
}

// GetNamespace return a namespace specified with the name
func GetNamespace(name string) *corev1.Namespace {

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return namespace
}
