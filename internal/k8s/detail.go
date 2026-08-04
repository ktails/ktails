package k8s

import (
	"context"
	"fmt"
	"sort"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// EventInfo is a condensed view of a Kubernetes event relevant to a resource.
type EventInfo struct {
	Type    string
	Reason  string
	Message string
	Age     string
	Count   int32
}

// ResourceDetail is a kind-agnostic bundle of everything the Detail tab
// renders for a single resource: a one-line summary, status conditions,
// recent events, and the resource's YAML.
type ResourceDetail struct {
	Kind      string // "Deployment", "Pod", ...
	Name      string
	Namespace string
	Age       string
	Summary   string // e.g. "Ready Replicas: 2" or "Phase: Running  Restarts: 3"
	Status    []string
	Events    []EventInfo
	YAML      string
}

// detailObject is any k8s API object Get*Detail renders — every concrete
// type this package calls renderDetailYAML on (*corev1.Pod,
// *appsv1.Deployment, ...) already implements both halves via their
// generated ObjectMeta/TypeMeta embedding, with no extra work needed at
// the call site.
type detailObject interface {
	runtime.Object
	metav1Object
}

// metav1Object avoids importing metav1 under two different names in this
// file (it's already aliased v1 here, unlike every other file in this
// package) — SetManagedFields is metav1.Object's only method
// renderDetailYAML needs.
type metav1Object interface {
	SetManagedFields(managedFields []v1.ManagedFieldsEntry)
}

// renderDetailYAML clears ManagedFields (pure noise for a human reader),
// sets TypeMeta (List/Get results omit it — client-go's decoder doesn't
// need it, but yaml.Marshal has no other way to know apiVersion/kind), and
// renders obj as YAML the way `kubectl get -o yaml` would. Shared by every
// Get*Detail function's YAML tail, which was otherwise byte-identical
// across all 13 kinds. Never returns an error — a Marshal failure renders
// as an inline message instead, matching every call site's own existing
// fallback.
func renderDetailYAML(obj detailObject, apiVersion, kind string) string {
	obj.SetManagedFields(nil)
	obj.GetObjectKind().SetGroupVersionKind(schema.FromAPIVersionAndKind(apiVersion, kind))
	yamlBytes, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Sprintf("failed to render YAML: %v", err)
	}
	return string(yamlBytes)
}

// getEvents fetches events for a specific object, newest first.
func (c *Client) getEvents(kubeContextName, namespace, kind, name string) ([]EventInfo, error) {
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	fieldSelector := fmt.Sprintf("involvedObject.name=%s,involvedObject.namespace=%s,involvedObject.kind=%s",
		name, namespace, kind)
	eventList, err := clientset.CoreV1().Events(namespace).List(ctx, v1.ListOptions{FieldSelector: fieldSelector})
	if err != nil {
		return nil, err
	}

	sort.Slice(eventList.Items, func(i, j int) bool {
		return eventList.Items[i].LastTimestamp.After(eventList.Items[j].LastTimestamp.Time)
	})

	events := make([]EventInfo, 0, len(eventList.Items))
	for _, ev := range eventList.Items {
		events = append(events, EventInfo{
			Type:    ev.Type,
			Reason:  ev.Reason,
			Message: ev.Message,
			Age:     formatDuration(time.Since(ev.LastTimestamp.Time)),
			Count:   ev.Count,
		})
	}

	return events, nil
}
