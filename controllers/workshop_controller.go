/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"regexp"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/prometheus/common/log"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workshopv1 "github.com/stakater/workshop-operator/api/v1"
	"github.com/stakater/workshop-operator/common/util"
)

// WorkshopReconciler reconciles a Workshop object
type WorkshopReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// Finalizer
const workshopFinalizer = "finalizer.workshop.stakater.com"

// +kubebuilder:rbac:groups=workshop.stakater.com,resources=workshops;workshops/finalizers,verbs=*
// +kubebuilder:rbac:groups=workshop.stakater.com,resources=workshops/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=workshop.stakater.com,resources=workshops,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=create;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=pods;services;endpoints;persistentvolumeclaims;events;configmaps;secrets;namespaces;serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=create;list;watch;update;patch;get;delete
// +kubebuilder:rbac:groups=project.openshift.io,resources=projectrequests,verbs=create

// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterroles;clusterrolebindings,verbs=*
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=org.eclipse.che,resources=checlusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=maistra.io,resources=servicemeshcontrolplanes;servicemeshmemberrolls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations;validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gpte.opentlc.com,resources=nexus;giteas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operators.coreos.com,resources=operatorgroups;subscriptions;clusterserviceversions;installplans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=argoproj.io,resources=argocds;appprojects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kiali.io,resources=kialis,verbs=get;list;watch;patch

func (r *WorkshopReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	reqLogger := r.Log.WithValues("workshop", req.NamespacedName)

	// Fetch the Workshop workshop
	workshop := &workshopv1.Workshop{}
	err := r.Get(ctx, req.NamespacedName, workshop)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("Workshop resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Workshop")
		return reconcile.Result{}, err
	}

	//////////////////////////
	// Variables
	//////////////////////////
	var (
		openshiftConsoleURL string
		appsHostnameSuffix  string
	)
	// extract app route suffix from openshift-console
	route := &routev1.Route{}
	if err := r.Get(ctx, types.NamespacedName{Name: "console", Namespace: "openshift-console"}, route); err != nil {
		log.Errorf("Failed to get OpenShift Console: %s", err)
		return reconcile.Result{}, err
	}
	openshiftConsoleURL = "https://" + route.Spec.Host
	log.Infof("OpenShift Console URL %s", openshiftConsoleURL)

	re := regexp.MustCompile(`^console-openshift-console.(.*?)$`)
	match := re.FindStringSubmatch(route.Spec.Host)
	appsHostnameSuffix = match[1]
	log.Infof("Apps Hostname Suffix %s", appsHostnameSuffix)

	users := workshop.Spec.User.Number
	if users < 0 {
		users = 0
	}
	// Handle Cleanup on Deletion

	// Check if the Workshop workshop is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isWorkshopMarkedToBeDeleted := workshop.GetDeletionTimestamp() != nil
	if isWorkshopMarkedToBeDeleted {
		if util.Contains(workshop.GetFinalizers(), workshopFinalizer) {
			// Run finalization logic for workshopFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := r.finalizeWorkshop(reqLogger, workshop); err != nil {
				return ctrl.Result{}, err
			}
			_, _ = r.handleDelete(ctx, req, workshop, users, appsHostnameSuffix, openshiftConsoleURL)
			// Remove workshopFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(workshop, workshopFinalizer)
			log.Info("Finalizer removed for workshop" + workshop.ObjectMeta.Name)
			err := r.Update(ctx, workshop)
			if err != nil {
				return ctrl.Result{}, err
			}

		}
		return ctrl.Result{}, nil
	}

	// Add finalizer for this CR
	if !util.Contains(workshop.GetFinalizers(), workshopFinalizer) {
		if err := r.addFinalizer(reqLogger, workshop); err != nil {
			return ctrl.Result{}, err
		}
	}
	//////////////////////////
	// Portal
	//////////////////////////
	if result, err := r.reconcilePortal(workshop, users, appsHostnameSuffix, openshiftConsoleURL); util.IsRequeued(result, err) {
		return result, err
	}

	//////////////////////////
	// Projects
	//////////////////////////
	if result, err := r.reconcileProject(workshop, users); util.IsRequeued(result, err) {
		return result, err
	}

	//////////////////////////
	// Bookbag
	//////////////////////////
	if result, err := r.reconcileBookbag(workshop, users, appsHostnameSuffix, openshiftConsoleURL); util.IsRequeued(result, err) {
		return result, err
	}

	//////////////////////////
	// Nexus
	//////////////////////////
	if result, err := r.reconcileNexus(workshop); util.IsRequeued(result, err) {
		return result, err
	}

	//////////////////////////
	// Gitea
	//////////////////////////
	if result, err := r.reconcileGitea(workshop, users); util.IsRequeued(result, err) {
		return result, err
	}

	//////////////////////////
	// Pipeline
	//////////////////////////
	if result, err := r.reconcilePipelines(workshop); util.IsRequeued(result, err) {
		return result, err
	}

	//////////////////////////
	// GitOps
	//////////////////////////
	if result, err := r.reconcileGitOps(workshop, users, appsHostnameSuffix, openshiftConsoleURL); util.IsRequeued(result, err) {
		return result, err
	}

	//////////////////////////
	// CodeReadyWorkspace
	//////////////////////////
	if result, err := r.reconcileCodeReadyWorkspace(workshop, users, appsHostnameSuffix, openshiftConsoleURL); util.IsRequeued(result, err) {
		return result, err
	}

	//////////////////////////
	// Service Mesh
	//////////////////////////
	if result, err := r.reconcileServiceMesh(workshop, users); util.IsRequeued(result, err) {
		return result, err
	}

	//////////////////////////
	// Serverless
	//////////////////////////
	if result, err := r.reconcileServerless(workshop); util.IsRequeued(result, err) {
		return result, err
	}

	//////////////////////////
	// Vault
	//////////////////////////
	if result, err := r.reconcileVault(workshop, users); util.IsRequeued(result, err) {
		return result, err
	}

	//////////////////////////
	// Cert Manager
	//////////////////////////
	if result, err := r.reconcileCertManager(workshop, users); util.IsRequeued(result, err) {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *WorkshopReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workshopv1.Workshop{}).
		Complete(r)
}

func (r *WorkshopReconciler) handleDelete(ctx context.Context, req ctrl.Request, workshop *workshopv1.Workshop, userID int, appsHostnameSuffix string, openshiftConsoleURL string) (ctrl.Result, error) {
	log := r.Log.WithValues("workshop", req.NamespacedName)
	log.Info("Deleting workshop   " + workshop.ObjectMeta.Name)

	if result, err := r.deleteServiceMeshService(workshop, userID); util.IsRequeued(result, err) {
		return result, err
	}

	if result, err := r.deleteBookbag(workshop, userID, appsHostnameSuffix, openshiftConsoleURL); util.IsRequeued(result, err) {

		return result, err
	}

	if result, err := r.deletePipelines(workshop); util.IsRequeued(result, err) {

		return result, err
	}

	if result, err := r.deleteGitOps(workshop, userID, appsHostnameSuffix, openshiftConsoleURL); util.IsRequeued(result, err) {
		return result, err
	}

	if result, err := r.deleteProject(workshop, userID); util.IsRequeued(result, err) {
		return result, err
	}

	if result, err := r.deleteCodeReadyWorkspace(workshop, userID, appsHostnameSuffix); util.IsRequeued(result, err) {
		return result, err
	}

	if result, err := r.deletePortal(workshop, userID, appsHostnameSuffix, openshiftConsoleURL); util.IsRequeued(result, err) {
		return result, err
	}

	if result, err := r.deleteVault(workshop); util.IsRequeued(result, err) {
		return result, err
	}

	if result, err := r.deleteGitea(workshop); util.IsRequeued(result, err) {
		return result, err
	}

	if result, err := r.deleteNexus(workshop); util.IsRequeued(result, err) {
		return result, err
	}

	return ctrl.Result{}, nil
}
