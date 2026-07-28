package k8s

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/yaml"
)

// IngressInfo contains Ingress metadata.
type IngressInfo struct {
	Name      string
	Namespace string
	Age       string
	Hosts     []string
	Class     string
	Backends  []string
}

// IngressToIngressInfo converts an Ingress object to IngressInfo.
func IngressToIngressInfo(ing *networkingv1.Ingress) IngressInfo {
	hosts := make([]string, 0, len(ing.Spec.Rules))
	for _, rule := range ing.Spec.Rules {
		if rule.Host != "" {
			hosts = append(hosts, rule.Host)
		}
	}

	class := ""
	if ing.Spec.IngressClassName != nil {
		class = *ing.Spec.IngressClassName
	}

	var backends []string
	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
		backends = append(backends, formatIngressBackend(ing.Spec.DefaultBackend.Service))
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service != nil {
				backends = append(backends, formatIngressBackend(path.Backend.Service))
			}
		}
	}

	return IngressInfo{
		Name:      ing.Name,
		Namespace: ing.Namespace,
		Age:       formatDuration(time.Since(ing.CreationTimestamp.Time)),
		Hosts:     hosts,
		Class:     class,
		Backends:  backends,
	}
}

func formatIngressBackend(svc *networkingv1.IngressServiceBackend) string {
	port := svc.Port.Name
	if port == "" {
		port = strconv.Itoa(int(svc.Port.Number))
	}
	return svc.Name + ":" + port
}

// WatchIngresses opens a watch on Ingresses in the given namespace. See
// WatchPods for the implicit list-then-watch behavior.
func (c *Client) WatchIngresses(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	w, err := clientset.NetworkingV1().Ingresses(namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to watch ingresses in namespace %s (context %s): %w", namespace, kubeContext, err)
	}
	return w, nil
}

// ListIngresses fetches every Ingress in the given namespace in one call.
// See ListPods for why this exists alongside the watch.
func (c *Client) ListIngresses(ctx context.Context, kubeContext, namespace string) ([]*networkingv1.Ingress, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	list, err := clientset.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list ingresses in namespace %s (context %s): %w", namespace, kubeContext, err)
	}
	out := make([]*networkingv1.Ingress, len(list.Items))
	for i := range list.Items {
		out[i] = &list.Items[i]
	}
	return out, nil
}

// GetIngressDetail fetches a single Ingress's rules, rendered YAML, and
// recent events.
func (c *Client) GetIngressDetail(kubeContextName, namespace, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "Ingress"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	ing, err := clientset.NetworkingV1().Ingresses(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get ingress %s in namespace %s (context %s): %w", name, namespace, kubeContextName, err)
	}

	info := IngressToIngressInfo(ing)
	d.Name = info.Name
	d.Namespace = info.Namespace
	d.Age = info.Age
	d.Summary = fmt.Sprintf("Hosts: %s", strings.Join(info.Hosts, ", "))

	ing.ManagedFields = nil
	ing.TypeMeta = metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"}
	if yamlBytes, yamlErr := yaml.Marshal(ing); yamlErr == nil {
		d.YAML = string(yamlBytes)
	} else {
		d.YAML = fmt.Sprintf("failed to render YAML: %v", yamlErr)
	}

	if events, err := c.getEvents(kubeContextName, namespace, "Ingress", name); err == nil {
		d.Events = events
	}

	return d, nil
}
