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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	authcontroller "github.com/intel/authservice-configurator/api/v1"
	authcontrollerv1 "github.com/intel/authservice-configurator/api/v1"
)

// ChainReconciler reconciles a Chain object
type ChainReconciler struct {
	client.Client
	Log                       logr.Logger
	Scheme                    *runtime.Scheme
	Threads                   int
	AuthserviceDeploymentName string
}

// +kubebuilder:rbac:groups=authcontroller.intel.com,resources=chains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=authcontroller.intel.com,resources=chains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;create;update;patch;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;create;update;patch;watch
// +kubebuilder:rbac:groups=security.istio.io,resources=requestauthentications,verbs=get;list;create;update;patch;watch

// Reconcile creates/updates the AuthService configuration when a Chain is modified.
func (r *ChainReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logger := r.Log.WithValues("chain", req.NamespacedName)

	// Get the updated chain.
	var chain authcontroller.Chain
	if err := r.Get(ctx, req.NamespacedName, &chain); err != nil {
		logger.Error(err, "Chain not found, ignoring")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	chains, err := getAllChains(r, logger, req.NamespacedName.Namespace)
	if err != nil {
		logger.Error(err, "Failed to get all chains")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Generate the ConfigMap based on the configuration and the chains.
	configMap, update := createConfigMap(r, req.NamespacedName.Namespace, r.Threads, chains)

	// Create/Update the existing ConfigMap if it exists with the new JSON file.
	if update {
		if err := r.Update(ctx, configMap); err != nil {
			logger.Error(err, "Failed to update ConfigMap")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	} else {
		if err := r.Create(ctx, configMap); err != nil {
			logger.Error(err, "Failed to create ConfigMap")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	for _, chain := range chains.Items {
		if err := createRequestAuthentication(r, logger, &chain); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	if err := restartAuthService(r, logger, r.AuthserviceDeploymentName, req.Namespace); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager connects the controller with the manager.
func (r *ChainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&authcontrollerv1.Chain{}).
		Complete(r)
}
