package controller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SecretReconciler reconciles a Secret object
type SecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

func (r *SecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Secret that triggered the event
	secret := &corev1.Secret{}
	if err := r.Get(ctx, req.NamespacedName, secret); err != nil {
		if errors.IsNotFound(err) {
			// Secret was deleted, no action needed
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch Secret")
		return ctrl.Result{}, err
	}

	// Find Deployments that reference this Secret
	deployments := &appsv1.DeploymentList{}
	if err := r.List(
		ctx,
		deployments,
		client.InNamespace(req.Namespace),
		client.MatchingFields{"spec.template.spec.secretRefs": secret.Name},
	); err != nil {
		log.Error(err, "Failed to list Deployments")
		return ctrl.Result{}, err
	}

	// Trigger a reload for each Deployment
	for _, deployment := range deployments.Items {
		if err := r.triggerReload(ctx, &deployment); err != nil {
			log.Error(err, "Failed to trigger reload", "deployment", deployment.Name)
			return ctrl.Result{}, err
		}
		log.Info("Triggered reload for Deployment", "deployment", deployment.Name)
	}

	return ctrl.Result{}, nil
}

// triggerReload updates the Deployment's pod template to trigger a rolling update
func (r *SecretReconciler) triggerReload(ctx context.Context, deployment *appsv1.Deployment) error {
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations["myoperator.com/reload-timestamp"] = time.Now().String()
	return r.Update(ctx, deployment)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Index Deployments by referenced Secrets
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&appsv1.Deployment{},
		"spec.template.spec.secretRefs",
		indexSecretRefs,
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		Complete(r)
}

// indexSecretRefs indexes Deployments by the Secrets they reference
func indexSecretRefs(obj client.Object) []string {
	deployment := obj.(*appsv1.Deployment)
	secretSet := make(map[string]struct{})

	// Check volumes
	for _, vol := range deployment.Spec.Template.Spec.Volumes {
		if vol.Secret != nil {
			secretSet[vol.Secret.SecretName] = struct{}{}
		}
	}

	// Check environment variables and envFrom
	for _, container := range deployment.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				secretSet[env.ValueFrom.SecretKeyRef.Name] = struct{}{}
			}
		}
		for _, envFrom := range container.EnvFrom {
			if envFrom.SecretRef != nil {
				secretSet[envFrom.SecretRef.Name] = struct{}{}
			}
		}
	}

	// Check init containers
	for _, container := range deployment.Spec.Template.Spec.InitContainers {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				secretSet[env.ValueFrom.SecretKeyRef.Name] = struct{}{}
			}
		}
		for _, envFrom := range container.EnvFrom {
			if envFrom.SecretRef != nil {
				secretSet[envFrom.SecretRef.Name] = struct{}{}
			}
		}
	}

	// Convert set to slice
	secrets := make([]string, 0, len(secretSet))
	for secret := range secretSet {
		secrets = append(secrets, secret)
	}

	return secrets
}
