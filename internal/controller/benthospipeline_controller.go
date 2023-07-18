package controller

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	streamv1 "github.com/charlie-haley/benthos-operator/api/v1alpha1"
	"github.com/charlie-haley/benthos-operator/internal/pkg/resource"
)

// BenthosPipelineReconciler reconciles a BenthosPipeline object
type BenthosPipelineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type PipelineScope struct {
	Log      logr.Logger
	Ctx      context.Context
	Client   client.Client
	Pipeline *streamv1.BenthosPipeline
}

// +kubebuilder:rbac:groups=streaming.benthos.dev,resources=benthospipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=streaming.benthos.dev,resources=benthospipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=streaming.benthos.dev,resources=benthospipelines/finalizers,verbs=update
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the main reconcile loop for the Benthos Pipeline
func (r *BenthosPipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// fetch BenthosPipeline
	pipeline := &streamv1.BenthosPipeline{}
	err := r.Get(ctx, req.NamespacedName, pipeline)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	scope := &PipelineScope{
		Log:      log,
		Ctx:      ctx,
		Client:   r.Client,
		Pipeline: pipeline,
	}

	// handle pipeline deletion
	if !pipeline.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(scope)
	}

	// handle pipeline reconcile
	return r.reconcileNormal(scope)
}

// reconcileNormal handles normal reconciles
func (r *BenthosPipelineReconciler) reconcileNormal(scope *PipelineScope) (ctrl.Result, error) {
	// add finalizer to the BenthosPipeline
	controllerutil.AddFinalizer(scope.Pipeline, streamv1.PipelineFinalizer)

	// check if the Pipeline has already created a deployment
	_, err := r.upsertDeployment(scope)
	if err != nil {
		return reconcile.Result{}, err
	}

	// set status
	_, err = r.setPipelineStatus(scope)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// reconcileNormal handles a deletion during the reconcile
func (r *BenthosPipelineReconciler) reconcileDelete(scope *PipelineScope) (ctrl.Result, error) {
	// remove finalizer to allow the resource to be deleted
	controllerutil.RemoveFinalizer(scope.Pipeline, streamv1.PipelineFinalizer)

	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BenthosPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&streamv1.BenthosPipeline{}).
		Complete(r)
}

// TODO: move to a shared utils
func (r *BenthosPipelineReconciler) upsertDeployment(scope *PipelineScope) (ctrl.Result, error) {
	pipeline := scope.Pipeline
	replicas := pipeline.Spec.Replicas

	_, err := r.createOrUpdateConfigMap(scope, "streams.yaml", pipeline.Spec.Config)
	if err != nil {
		return reconcile.Result{}, err
	}

	_, err = r.createOrUpdateDeployment(scope)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Fetch deployment to get replicas
	deployment := &appsv1.Deployment{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: pipeline.Name, Namespace: pipeline.Namespace}, deployment)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to get deployment", "deployment", pipeline.Name)

	}

	scope.Pipeline.Status.AvailableReplicas = deployment.Status.AvailableReplicas
	if deployment.Status.ReadyReplicas == replicas {
		scope.status(true, "Running")
		return reconcile.Result{}, nil
	}
	if deployment.Status.ReadyReplicas > replicas {
		scope.status(true, "Scaling down")
		return reconcile.Result{}, nil
	}
	if deployment.Status.UnavailableReplicas == replicas {
		scope.status(false, "Deployment failed")
		return reconcile.Result{}, nil
	}
	if deployment.Status.ReadyReplicas < replicas {
		scope.status(true, "Scaling up")
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func (ps *PipelineScope) status(ready bool, phase string) {
	ps.Pipeline.Status.Ready = ready
	ps.Pipeline.Status.Phase = phase
}

func (r *BenthosPipelineReconciler) createOrUpdateDeployment(scope *PipelineScope) (ctrl.Result, error) {
	name := scope.Pipeline.Name
	namespace := scope.Pipeline.Namespace
	replicas := scope.Pipeline.Spec.Replicas

	dp := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	op, err := controllerutil.CreateOrUpdate(scope.Ctx, scope.Client, dp, func() error {
		template := resource.NewDeployment(name, namespace, replicas)

		// Deployment selector is immutable we only change this value if we're creating a new resource.
		if dp.CreationTimestamp.IsZero() {
			dp.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: template.Spec.Selector.MatchLabels,
			}
		}

		// Update the template
		dp.Spec.Template = template.Spec.Template
		dp.Spec.Replicas = template.Spec.Replicas
		controllerutil.SetControllerReference(dp.GetObjectMeta(), dp, r.Scheme)
		return nil
	})

	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "Failed to reconcile config map", name)
	}
	scope.Log.V(5).Info("Succesfully reconciled config map", name, "operation", op)
	return reconcile.Result{}, nil
}

// createOrUpdateConfigMap updates a config map or creates it if it doesn't exist
func (r *BenthosPipelineReconciler) createOrUpdateConfigMap(scope *PipelineScope, key string, data string) (ctrl.Result, error) {
	name := scope.Pipeline.Name

	sc := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "benthos-streams" + name, Namespace: scope.Pipeline.Namespace}}
	op, err := controllerutil.CreateOrUpdate(scope.Ctx, scope.Client, sc, func() error {
		sc.Data = map[string]string{
			key: data,
		}
		controllerutil.SetControllerReference(sc.GetObjectMeta(), sc, r.Scheme)
		return nil
	})

	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "Failed to reconcile config map", name)
	}
	scope.Log.V(5).Info("Succesfully reconciled config map", name, "operation", op)

	return reconcile.Result{}, nil
}

func (r *BenthosPipelineReconciler) setPipelineStatus(scope *PipelineScope) (ctrl.Result, error) {
	pipeline := scope.Pipeline
	scope.Log.Info("Setting BenthosPipeline status.", "ready", pipeline.Status.Ready, "phase", pipeline.Status.Phase)

	err := r.Status().Update(context.Background(), pipeline)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}
