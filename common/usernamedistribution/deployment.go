package usernamedistribution

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/prometheus/common/log"
	workshopv1 "github.com/stakater/workshop-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

// NewDeployment create a deployment
func NewDeployment(workshop *workshopv1.Workshop, scheme *runtime.Scheme,
	name string, labels map[string]string, redisServiceName string, users int,
	appsHostnameSuffix string, openshiftConsoleURL string) *appsv1.Deployment {

	image := "quay.io/mcouliba/username-distribution:latest"
	labModuleURLs := "https://docs.openshift.com/container-platform/latest/welcome/index.html;openshift_docs"
	guideURLParameters := "APPS_HOSTNAME_SUFFIX=" + appsHostnameSuffix +
		"&USER_ID=%USER_ID%" +
		"&OPENSHIFT_PASSWORD=" + workshop.Spec.User.Password +
		"&WORKSHOP_GIT_REPO=" + url.QueryEscape(workshop.Spec.Source.GitURL) +
		"&WORKSHOP_GIT_REF=" + workshop.Spec.Source.GitBranch

	if workshop.Spec.Infrastructure.Guide.Scholars.Enabled {
		isFirst := true
		for guideName, guideURL := range workshop.Spec.Infrastructure.Guide.Scholars.GuideURL {
			if isFirst {
				labModuleURLs = fmt.Sprintf("%s?%s;%s", guideURL, guideURLParameters, guideName)
				isFirst = false
			} else {
				labModuleURLs = fmt.Sprintf("%s,%s?%s;%s", labModuleURLs, guideURL, guideURLParameters, guideName)
			}
		}
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: workshop.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: name,
							Env: []corev1.EnvVar{
								{
									Name:  "LAB_REDIS_HOST",
									Value: redisServiceName,
								},
								{
									Name:  "LAB_REDIS_PASS",
									Value: redisServiceName,
								},
								{
									Name:  "LAB_TITLE",
									Value: "OpenShift Workshops",
								},
								{
									Name:  "LAB_DURATION_HOURS",
									Value: "1week",
								},
								{
									Name:  "LAB_USER_COUNT",
									Value: strconv.Itoa(users),
								},
								{
									Name:  "LAB_USER_ACCESS_TOKEN",
									Value: workshop.Spec.User.Password,
								},
								{
									Name:  "LAB_USER_PASS",
									Value: workshop.Spec.User.Password,
								},
								{
									Name:  "LAB_USER_PREFIX",
									Value: "user",
								},
								{
									Name:  "LAB_USER_PAD_ZERO",
									Value: "false",
								},
								{
									Name:  "LAB_ADMIN_PASS",
									Value: "r3dh4t1!",
								},
								{
									Name:  "LAB_MODULE_URLS",
									Value: labModuleURLs,
								},
							},
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
									Protocol:      "TCP",
								},
							},
						},
					},
				},
			},
		},
	}

	// Set Workshop instance as the owner and controller
	err := ctrl.SetControllerReference(workshop, dep, scheme)
	if err != nil {
		log.Error(err, "Failed to set SetControllerReference")
	}
	return dep
}
