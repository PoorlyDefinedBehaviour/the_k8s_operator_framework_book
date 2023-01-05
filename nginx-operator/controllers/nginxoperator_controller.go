/*
Copyright 2023.

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
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/example/nginx-operator/api/v1alpha1"
	"github.com/example/nginx-operator/assets"
	"github.com/example/nginx-operator/controllers/metrics"
	"github.com/hashicorp/go-multierror"
)

// NginxOperatorReconciler reconciles a NginxOperator object
type NginxOperatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=operator.example.com,resources=nginxoperators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.example.com,resources=nginxoperators/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.example.com,resources=nginxoperators/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NginxOperator object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *NginxOperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	metrics.ReconcilesTotal.Inc()

	logger := log.FromContext(ctx)

	// condition, err := conditions.InClusterFactory{Client: r.Client}.
	// 	NewCondition(apiv2.ConditionType(apiv2.Upgradeable))
	// if err != nil {
	// 	return ctrl.Result{}, errors.WithStack(err)
	// }

	// if err := condition.Set(ctx, metav1.ConditionTrue,
	// 	conditions.WithReason("OperatorUpgradeable"),
	// 	conditions.WithMessage("The operator is upgradeable")); err != nil {
	// 	return ctrl.Result{}, errors.WithStack(err)
	// }

	operatorCustomResource := operatorv1alpha1.NginxOperator{}

	if err := r.Get(ctx, req.NamespacedName, &operatorCustomResource); err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Info("operator resource object not found")
			return ctrl.Result{}, nil
		} else {
			logger.Error(err, "error getting operator resource object")
			meta.SetStatusCondition(&operatorCustomResource.Status.Conditions, metav1.Condition{
				Type:               "OperatorDegraded",
				Status:             metav1.ConditionTrue,
				Reason:             "OperatorResourceNotAvailable",
				LastTransitionTime: metav1.NewTime(time.Now()),
				Message:            fmt.Sprintf("unable to get operator custom resource: %s", err.Error()),
			})
			return ctrl.Result{}, multierror.Append(err, r.Status().Update(ctx, &operatorCustomResource))
		}
	}

	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, req.NamespacedName, deployment); err != nil {
		if !k8serrors.IsNotFound(err) {
			logger.Error(err, "error getting existing nginx deployment")
			meta.SetStatusCondition(&operatorCustomResource.Status.Conditions, metav1.Condition{
				Type:               "OperatorDegraded",
				Status:             metav1.ConditionTrue,
				Reason:             "OperandDeploymentNotAvailable",
				LastTransitionTime: metav1.NewTime(time.Now()),
				Message:            fmt.Sprintf("unable to get existing nginx deployment: %s", err.Error()),
			})
			return ctrl.Result{}, multierror.Append(err, r.Status().Update(ctx, &operatorCustomResource))
		}

		deployment.Namespace = req.Namespace
		deployment.Name = req.Name
		deployment = assets.GetDeploymentFromFile("manifests/nginx_deployment.yaml")
	}

	if operatorCustomResource.Spec.Replicas != nil {
		deployment.Spec.Replicas = operatorCustomResource.Spec.Replicas
	}

	if operatorCustomResource.Spec.Port != nil {
		deployment.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort =
			*operatorCustomResource.Spec.Port
	}

	result, err := ctrl.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		return ctrl.SetControllerReference(&operatorCustomResource, deployment, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "error creating or updating deployment")
		meta.SetStatusCondition(&operatorCustomResource.Status.Conditions, metav1.Condition{
			Type:               "OperatorDegraded",
			Status:             metav1.ConditionTrue,
			Reason:             "OperandDeploymentNotAvailable",
			LastTransitionTime: metav1.NewTime(time.Now()),
			Message:            fmt.Sprintf("unable to update operand deployment: %s", err.Error()),
		})
		return ctrl.Result{}, multierror.Append(err, r.Status().Update(ctx, &operatorCustomResource))
	}

	logger.Info("controller created or updated", "result", result)
	meta.SetStatusCondition(&operatorCustomResource.Status.Conditions, metav1.Condition{
		Type:               "OperatorDegraded",
		Status:             metav1.ConditionFalse,
		Reason:             "OperatorSucceeded",
		LastTransitionTime: metav1.NewTime(time.Now()),
		Message:            "operator successfully reconciling",
	})

	return ctrl.Result{}, r.Status().Update(ctx, &operatorCustomResource)
}

// SetupWithManager sets up the controller with the Manager.
func (r *NginxOperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.NginxOperator{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
