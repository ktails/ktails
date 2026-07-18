package k8s

import (
	"context"
	"fmt"
	"sort"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// getEvents fetches events for a specific object, newest first.
func (c *Client) getEvents(kubeContextName, namespace, kind, name string) ([]EventInfo, error) {
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	fieldSelector := fmt.Sprintf("involvedObject.name=%s,involvedObject.namespace=%s,involvedObject.kind=%s",
		name, namespace, kind)
	eventList, err := clientset.CoreV1().Events(namespace).List(context.Background(), v1.ListOptions{FieldSelector: fieldSelector})
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
