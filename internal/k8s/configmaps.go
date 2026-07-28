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
	"sigs.k8s.io/yaml"
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

// WatchConfigMaps opens a watch on ConfigMaps in the given namespace. See
// WatchPods for the implicit list-then-watch behavior.
func (c *Client) WatchConfigMaps(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	w, err := clientset.CoreV1().ConfigMaps(namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to watch configmaps in namespace %s (context %s): %w", namespace, kubeContext, err)
	}
	return w, nil
}

// ListConfigMaps fetches every ConfigMap in the given namespace in one call.
// See ListPods for why this exists alongside the watch.
func (c *Client) ListConfigMaps(ctx context.Context, kubeContext, namespace string) ([]*corev1.ConfigMap, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	list, err := clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list configmaps in namespace %s (context %s): %w", namespace, kubeContext, err)
	}
	out := make([]*corev1.ConfigMap, len(list.Items))
	for i := range list.Items {
		out[i] = &list.Items[i]
	}
	return out, nil
}

// GetConfigMapDetail fetches a single ConfigMap's data keys, rendered YAML,
// and recent events.
func (c *Client) GetConfigMapDetail(kubeContextName, namespace, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "ConfigMap"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get configmap %s in namespace %s (context %s): %w", name, namespace, kubeContextName, err)
	}

	info := ConfigMapToConfigMapInfo(cm)
	d.Name = info.Name
	d.Namespace = info.Namespace
	d.Age = info.Age
	d.Summary = fmt.Sprintf("Keys: %s", strings.Join(info.Keys, ", "))

	cm.ManagedFields = nil
	cm.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}
	if yamlBytes, yamlErr := yaml.Marshal(cm); yamlErr == nil {
		d.YAML = string(yamlBytes)
	} else {
		d.YAML = fmt.Sprintf("failed to render YAML: %v", yamlErr)
	}

	if events, err := c.getEvents(kubeContextName, namespace, "ConfigMap", name); err == nil {
		d.Events = events
	}

	return d, nil
}
