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

// nodeRolePrefix labels a Node's role, e.g.
// "node-role.kubernetes.io/control-plane" — the role name is whatever
// follows the slash.
const nodeRolePrefix = "node-role.kubernetes.io/"

// NodeInfo contains Node metadata. Nodes are cluster-scoped — there is no
// namespace.
type NodeInfo struct {
	Name       string
	Status     string
	Roles      string
	Age        string
	Version    string
	InternalIP string
	OS         string
}

// nodeRoles extracts role names from a Node's node-role.kubernetes.io/*
// labels, sorted and comma-joined, "<none>" if it carries none (a worker
// node in most clusters).
func nodeRoles(labels map[string]string) string {
	var roles []string
	for label := range labels {
		if role, ok := strings.CutPrefix(label, nodeRolePrefix); ok {
			if role == "" {
				role = "master"
			}
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		return "<none>"
	}
	sort.Strings(roles)
	return strings.Join(roles, ",")
}

// nodeReadyStatus reports a Node's Ready condition as "Ready"/"NotReady",
// or "Unknown" if the condition is absent entirely.
func nodeReadyStatus(conditions []corev1.NodeCondition) string {
	for _, c := range conditions {
		if c.Type == corev1.NodeReady {
			if c.Status == corev1.ConditionTrue {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "Unknown"
}

// nodeInternalIP returns a Node's InternalIP address, empty if unset.
func nodeInternalIP(addresses []corev1.NodeAddress) string {
	for _, a := range addresses {
		if a.Type == corev1.NodeInternalIP {
			return a.Address
		}
	}
	return ""
}

// NodeToNodeInfo converts a Node object to NodeInfo.
func NodeToNodeInfo(node *corev1.Node) NodeInfo {
	return NodeInfo{
		Name:       node.Name,
		Status:     nodeReadyStatus(node.Status.Conditions),
		Roles:      nodeRoles(node.Labels),
		Age:        formatDuration(time.Since(node.CreationTimestamp.Time)),
		Version:    node.Status.NodeInfo.KubeletVersion,
		InternalIP: nodeInternalIP(node.Status.Addresses),
		OS:         node.Status.NodeInfo.OperatingSystem + "/" + node.Status.NodeInfo.Architecture,
	}
}

// nodesLW binds a NodeInterface's List/Watch, for watchResource/
// listResource — see generic.go. Nodes are cluster-scoped, so unlike every
// other kind's *LW helper this takes no namespace.
func nodesLW(cs kubernetes.Interface) listerWatcher[*corev1.NodeList] {
	return cs.CoreV1().Nodes()
}

// WatchNodes opens a watch on every Node in the cluster. namespace is
// ignored — Nodes are cluster-scoped, unlike every other watched kind.
func (c *Client) WatchNodes(ctx context.Context, kubeContext, _ string) (watch.Interface, error) {
	return watchResource(ctx, c, kubeContext, "nodes", nodesLW)
}

// ListNodes fetches every Node in the cluster in one call. namespace is
// ignored — Nodes are cluster-scoped. See ListPods for why this exists
// alongside the watch.
func (c *Client) ListNodes(ctx context.Context, kubeContext, _ string) ([]*corev1.Node, error) {
	return listResource(ctx, c, kubeContext, "nodes", nodesLW,
		func(l *corev1.NodeList) []*corev1.Node { return pointers(l.Items) })
}

// GetNodeDetail fetches a single Node's status, rendered YAML, and recent
// events. namespace is ignored — Nodes are cluster-scoped.
func (c *Client) GetNodeDetail(kubeContextName, _, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "Node"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	node, err := clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get node %s (context %s): %w", name, kubeContextName, err)
	}

	info := NodeToNodeInfo(node)
	d.Name = info.Name
	d.Age = info.Age
	d.Summary = fmt.Sprintf("Status: %s  Roles: %s  Version: %s  IP: %s", info.Status, info.Roles, info.Version, info.InternalIP)
	for _, condition := range node.Status.Conditions {
		d.Status = append(d.Status, formatCondition(string(condition.Type), string(condition.Status), condition.Reason, condition.Message))
	}
	d.YAML = renderDetailYAML(node, "v1", "Node")

	if events, err := c.getEvents(kubeContextName, "", "Node", name); err == nil {
		d.Events = events
	}

	return d, nil
}
