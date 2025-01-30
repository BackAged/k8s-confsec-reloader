package controller

import (
	"context"
	"strings"
	"time"

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

// ConfigMapReconciler reconciles a ConfigMap object
type ConfigMapReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the ConfigMap that triggered the event
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, configMap); err != nil {
		if errors.IsNotFound(err) {
			// ConfigMap was deleted, no action needed
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch ConfigMap")
		return ctrl.Result{}, err
	}

	// Find Deployments that reference this ConfigMap
	deployments := &appsv1.DeploymentList{}
	if err := r.List(
		ctx,
		deployments,
		client.InNamespace(req.Namespace),
		client.MatchingFields{IndexKey: configMap.Name},
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

// triggerReload updates the Deployment's pod template to trigger a update
func (r *ConfigMapReconciler) triggerReload(ctx context.Context, deployment *appsv1.Deployment) error {
	patch := client.MergeFrom(deployment.DeepCopy())

	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations[ReloadTimestampAnnotation] = time.Now().Format(time.RFC3339)

	// Use Patch instead of Update
	return r.Client.Patch(ctx, deployment, patch)
}

// indexConfigMapRefs indexes Deployments by the ConfigMaps they reference
func indexConfigMapRefs(obj client.Object) []string {
	deployment := obj.(*appsv1.Deployment)
	configMapSet := make(map[string]struct{})

	// Check volumes
	for _, vol := range deployment.Spec.Template.Spec.Volumes {
		if vol.ConfigMap != nil {
			configMapSet[vol.ConfigMap.Name] = struct{}{}
		}
	}

	// Check environment variables and envFrom
	for _, container := range deployment.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
				configMapSet[env.ValueFrom.ConfigMapKeyRef.Name] = struct{}{}
			}
		}
		for _, envFrom := range container.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				configMapSet[envFrom.ConfigMapRef.Name] = struct{}{}
			}
		}
	}

	// Check init containers
	for _, container := range deployment.Spec.Template.Spec.InitContainers {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
				configMapSet[env.ValueFrom.ConfigMapKeyRef.Name] = struct{}{}
			}
		}
		for _, envFrom := range container.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				configMapSet[envFrom.ConfigMapRef.Name] = struct{}{}
			}
		}
	}

	configMaps := make([]string, 0, len(configMapSet))
	for cm := range configMapSet {
		configMaps = append(configMaps, cm)
	}

	return configMaps
}

// process events based on following
// - configmap data is changed
// - configmap tracking is not disabled
func getConfigMapFilter() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			oldCm, okOld := event.ObjectOld.(*corev1.ConfigMap)
			newCm, okNew := event.ObjectNew.(*corev1.ConfigMap)

			if !okOld || !okNew {
				return false
			}

			if !parseWatch(newCm) {
				return false
			}

			keysToWatch := parseKeysToWatch(newCm)

			oldHash := GetConfigMapHash(oldCm, keysToWatch)
			newHash := GetConfigMapHash(newCm, keysToWatch)

			return oldHash != newHash
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},
	}
}

// parseKeysToWatch extracts the list of keys to watch from annotations
func parseKeysToWatch(cm *corev1.ConfigMap) []string {
	if val, exists := cm.Annotations[ConfigMapKeyWatchAnnotation]; exists {
		return strings.Split(val, ",")
	}

	return nil
}

// parseWatch extracts watch annotations
func parseWatch(cm *corev1.ConfigMap) bool {
	if val, exists := cm.Annotations[ConfigMapWatchAnnotation]; exists {
		return val == "true"
	}

	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Index Deployments by referenced ConfigMaps
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&appsv1.Deployment{},
		IndexKey,
		indexConfigMapRefs,
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(getConfigMapFilter()).
		Complete(r)
}
