package controller

import (
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	WatchAnnotation           string = "k8s-confsec-reloader.io/watch"
	KeyWatchAnnotation        string = "k8s-confsec-reloader.io/keys-to-watch"
	ReloadTimestampAnnotation string = "k8s-confsec-reloader.io/reload-timestamp"

	ConfigMapIndexKey string = "index.deployment.by.configmap"
	SecretIndexKey    string = "index.deployment.by.secret"
)

// parseWatch checks if the object is being tracked
// watch when annotation is empty or set to anything else except false
func parseWatch(obj client.Object) bool {
	return obj.GetAnnotations()[WatchAnnotation] != "false"
}

// parseKeysToWatch extracts keys to watch from the annotations
// watch all keys by default when no annotation is set
func parseKeysToWatch(obj client.Object) []string {
	keys := obj.GetAnnotations()[KeyWatchAnnotation]
	if keys == "" {
		return nil
	}

	return strings.Split(keys, ",")
}
