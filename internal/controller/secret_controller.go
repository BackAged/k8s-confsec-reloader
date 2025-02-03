package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
		client.MatchingFields{SecretIndexKey: secret.Name},
	); err != nil {
		log.Error(err, "Failed to list Deployments")
		return ctrl.Result{}, err
	}

	// Trigger a reload for each Deployment
	for _, deployment := range deployments.Items {
		if err := r.TriggerDeploymentReload(ctx, &deployment); err != nil {
			log.Error(err, "Failed to trigger reload", "deployment", deployment.Name)
			return ctrl.Result{}, err
		}
		log.Info("Triggered reload for Deployment", "deployment", deployment.Name)
	}

	return ctrl.Result{}, nil
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

	secrets := make([]string, 0, len(secretSet))
	for secret := range secretSet {
		secrets = append(secrets, secret)
	}

	return secrets
}

// process events based on following
// - secret data is changed
// - secret tracking is not disabled
func getSecretFilter() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return false // Ignore create events
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			oldSecret, okOld := event.ObjectOld.(*corev1.Secret)
			newSecret, okNew := event.ObjectNew.(*corev1.Secret)

			if !okOld || !okNew {
				return false
			}

			if !parseWatch(newSecret) {
				return false
			}

			// Get keys to watch (if specified)
			keysToWatch := parseKeysToWatch(newSecret)

			// Compare hashes of the old and new Secret data
			oldHash := GetSecretHash(oldSecret, keysToWatch)
			newHash := GetSecretHash(newSecret, keysToWatch)

			return oldHash != newHash
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return false // Ignore delete events
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Index Deployments by referenced Secrets
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&appsv1.Deployment{},
		SecretIndexKey,
		indexSecretRefs,
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		WithEventFilter(getSecretFilter()).
		Complete(r)
}
