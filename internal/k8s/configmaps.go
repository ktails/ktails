package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// ConfigMapInfo contains ConfigMap metadata.
type ConfigMapInfo struct {
	Name      string
	Namespace string
	Age       string
	Keys      []string
}

// ConfigMapToConfigMapInfo converts a ConfigMap object to ConfigMapInfo.
// Only key names are carried, never values — the table and Detail Pane show
// what's configured without dumping potentially large blobs into the row set.
func ConfigMapToConfigMapInfo(cm *corev1.ConfigMap) ConfigMapInfo {
	keys := make([]string, 0, len(cm.Data)+len(cm.BinaryData))
	for k := range cm.Data {
		keys = append(keys, k)
	}
	for k := range cm.BinaryData {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return ConfigMapInfo{
		Name:      cm.Name,
		Namespace: cm.Namespace,
		Age:       formatDuration(time.Since(cm.CreationTimestamp.Time)),
		Keys:      keys,
	}
}

// configMapsLW binds a ConfigMapInterface's List/Watch to namespace, for
// watchResource/listResource — see generic.go.
func configMapsLW(namespace string) func(kubernetes.Interface) listerWatcher[*corev1.ConfigMapList] {
	return func(cs kubernetes.Interface) listerWatcher[*corev1.ConfigMapList] {
		return cs.CoreV1().ConfigMaps(namespace)
	}
}

// WatchConfigMaps opens a watch on ConfigMaps in the given namespace. See
// WatchPods for the implicit list-then-watch behavior.
func (c *Client) WatchConfigMaps(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	return watchResource(ctx, c, kubeContext, "configmaps in namespace "+namespace, configMapsLW(namespace))
}

// ListConfigMaps fetches every ConfigMap in the given namespace in one call.
// See ListPods for why this exists alongside the watch.
func (c *Client) ListConfigMaps(ctx context.Context, kubeContext, namespace string) ([]*corev1.ConfigMap, error) {
	return listResource(ctx, c, kubeContext, "configmaps in namespace "+namespace, configMapsLW(namespace),
		func(l *corev1.ConfigMapList) []*corev1.ConfigMap { return pointers(l.Items) })
}

// GetConfigMapDetail fetches a single ConfigMap's data keys, rendered YAML,
// and recent events.
func (c *Client) GetConfigMapDetail(kubeContextName, namespace, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "ConfigMap"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get configmap %s in namespace %s (context %s): %w", name, namespace, kubeContextName, err)
	}

	info := ConfigMapToConfigMapInfo(cm)
	d.Name = info.Name
	d.Namespace = info.Namespace
	d.Age = info.Age
	d.Summary = fmt.Sprintf("Keys: %s", strings.Join(info.Keys, ", "))
	d.YAML = renderDetailYAML(cm, "v1", "ConfigMap")

	c.attachEvents(&d, kubeContextName, namespace, "ConfigMap", name)

	return d, nil
}
