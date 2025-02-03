package controller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TriggerDeploymentReload updates the Deployment's pod template to trigger a update
func (r *ConfigMapReconciler) TriggerDeploymentReload(ctx context.Context, deployment *appsv1.Deployment) error {
	patch := client.MergeFrom(deployment.DeepCopy())

	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations[ReloadTimestampAnnotation] = time.Now().Format(time.RFC3339)

	// Use Patch instead of Update
	return r.Patch(ctx, deployment, patch)
}

// TriggerDeploymentReload updates the Deployment's pod template to trigger a update
func (r *SecretReconciler) TriggerDeploymentReload(ctx context.Context, deployment *appsv1.Deployment) error {
	patch := client.MergeFrom(deployment.DeepCopy())

	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations[ReloadTimestampAnnotation] = time.Now().Format(time.RFC3339)

	// Use Patch instead of Update
	return r.Patch(ctx, deployment, patch)
}
