package k8s

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// ServiceInfo contains service metadata
type ServiceInfo struct {
	Name       string
	Namespace  string
	Type       string
	ClusterIP  string
	Ports      string
	Age        string
	Selector   string
	ExternalIP string
}

// GetServiceInfo retrieves service information for a specific context and namespace
func (c *Client) GetServiceInfo(kubeContextName, namespace string) ([]ServiceInfo, error) {
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	serviceList, err := clientset.CoreV1().Services(namespace).List(context.Background(), v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services in namespace %s (context %s): %w",
			namespace, kubeContextName, err)
	}

	serviceInfoList := make([]ServiceInfo, 0, len(serviceList.Items))
	for _, svc := range serviceList.Items {
		serviceInfoList = append(serviceInfoList, ServiceInfo{
			Name:       svc.Name,
			Namespace:  svc.Namespace,
			Type:       string(svc.Spec.Type),
			ClusterIP:  svc.Spec.ClusterIP,
			Ports:      formatServicePorts(svc.Spec.Ports),
			Age:        formatDuration(time.Since(svc.CreationTimestamp.Time)),
			Selector:   formatSelector(svc.Spec.Selector),
			ExternalIP: formatLoadBalancerIngress(svc.Status.LoadBalancer.Ingress),
		})
	}

	return serviceInfoList, nil
}

// GetServiceEndpoints lists EndpointSlices for the whole namespace in one
// call and groups their addresses by owning service name (via the
// kubernetes.io/service-name label EndpointSlices carry), so callers don't
// need one API round trip per service.
func (c *Client) GetServiceEndpoints(kubeContextName, namespace string) (map[string][]string, error) {
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	sliceList, err := clientset.DiscoveryV1().EndpointSlices(namespace).List(context.Background(), v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list endpoint slices in namespace %s (context %s): %w",
			namespace, kubeContextName, err)
	}

	endpoints := make(map[string][]string)
	for _, slice := range sliceList.Items {
		svcName, ok := slice.Labels[discoveryv1.LabelServiceName]
		if !ok {
			continue
		}
		for _, ep := range slice.Endpoints {
			endpoints[svcName] = append(endpoints[svcName], ep.Addresses...)
		}
	}

	return endpoints, nil
}

// GetServiceDetail fetches a single service's status, rendered YAML, and recent events.
func (c *Client) GetServiceDetail(kubeContextName, namespace, serviceName string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "Service"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	svc, err := clientset.CoreV1().Services(namespace).Get(context.Background(), serviceName, v1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get service %s in namespace %s (context %s): %w",
			serviceName, namespace, kubeContextName, err)
	}

	d.Name = svc.Name
	d.Namespace = svc.Namespace
	d.Age = formatDuration(time.Since(svc.CreationTimestamp.Time))
	d.Summary = fmt.Sprintf("Type: %s  ClusterIP: %s  Ports: %s", svc.Spec.Type, svc.Spec.ClusterIP, formatServicePorts(svc.Spec.Ports))

	for _, condition := range svc.Status.Conditions {
		d.Status = append(d.Status, formatCondition(condition.Type, string(condition.Status), condition.Reason, condition.Message))
	}
	if externalIP := formatLoadBalancerIngress(svc.Status.LoadBalancer.Ingress); externalIP != "" {
		d.Status = append(d.Status, fmt.Sprintf("LoadBalancer Ingress: %s", externalIP))
	}

	svc.ManagedFields = nil
	svc.TypeMeta = v1.TypeMeta{APIVersion: "v1", Kind: "Service"}
	if yamlBytes, yamlErr := yaml.Marshal(svc); yamlErr == nil {
		d.YAML = string(yamlBytes)
	} else {
		d.YAML = fmt.Sprintf("failed to render YAML: %v", yamlErr)
	}

	if events, err := c.getEvents(kubeContextName, namespace, "Service", serviceName); err == nil {
		d.Events = events
	}

	return d, nil
}

// formatSelector renders a service's label selector as a sorted, deterministic
// "key=value,key=value" string, empty for headless/selector-less services.
func formatSelector(selector map[string]string) string {
	if len(selector) == 0 {
		return ""
	}
	keys := make([]string, 0, len(selector))
	for k := range selector {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+selector[k])
	}
	return strings.Join(parts, ",")
}

// formatLoadBalancerIngress joins a LoadBalancer service's ingress
// hostnames/IPs (hostname preferred when both are set), empty when there's
// no ingress yet.
func formatLoadBalancerIngress(ingress []corev1.LoadBalancerIngress) string {
	if len(ingress) == 0 {
		return ""
	}
	var addrs []string
	for _, lb := range ingress {
		if lb.Hostname != "" {
			addrs = append(addrs, lb.Hostname)
		} else if lb.IP != "" {
			addrs = append(addrs, lb.IP)
		}
	}
	return strings.Join(addrs, ", ")
}

// formatServicePorts renders a service's ports as a compact "80:30080/TCP" list.
func formatServicePorts(ports []corev1.ServicePort) string {
	if len(ports) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		s := strconv.Itoa(int(p.Port))
		if p.NodePort != 0 {
			s += ":" + strconv.Itoa(int(p.NodePort))
		}
		s += "/" + string(p.Protocol)
		parts = append(parts, s)
	}
	return strings.Join(parts, ",")
}
