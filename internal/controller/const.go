package controller

const (
	ConfigMapWatchAnnotation    string = "k8s-confsec-reloader.io/watch"
	ConfigMapKeyWatchAnnotation string = "k8s-confsec-reloader.io/keys-to-watch"
	ReloadTimestampAnnotation   string = "k8s-confsec-reloader.io/reload-timestamp"

	IndexKey string = "index.deployment.by.configmap"
)
