// Package cmds builds the tea.Cmds that bridge k8s.Client calls into
// bubbletea's async update loop.
package cmds

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
)

// LoadServiceEndpointsCmd fetches Endpoint IPs for every service in one
// context+namespace via a single EndpointSlices list call. It's triggered
// lazily (see mainPage.go's Ctrl+W handling for the svc tab), independent of
// the Services watch, so it only ever runs once per context+namespace until
// that namespace's selection changes.
func LoadServiceEndpointsCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace string) tea.Cmd {
	return func() tea.Msg {
		endpoints, err := client.GetServiceEndpoints(ctx, kubeContext, namespace)
		if err != nil {
			return msgs.ServiceEndpointsMsg{Context: kubeContext, Namespace: namespace, Err: err}
		}
		return msgs.ServiceEndpointsMsg{Context: kubeContext, Namespace: namespace, Endpoints: endpoints}
	}
}

// LoadPodMetricsCmd fetches current CPU/Memory usage for every pod in one
// context+namespace from the Metrics Server. Triggered periodically while
// the Pods tab is active — see MainPage.fetchMetricsIfNeeded — independent
// of the Pods watch, since metrics.k8s.io has no watch support and must be
// polled.
func LoadPodMetricsCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace string) tea.Cmd {
	return func() tea.Msg {
		infos, err := client.ListPodMetrics(ctx, kubeContext, namespace)
		if err != nil {
			return msgs.PodMetricsMsg{Context: kubeContext, Err: err}
		}
		usage := make(map[string]msgs.ResourceUsage, len(infos))
		for _, info := range infos {
			usage[info.Namespace+"/"+info.Name] = msgs.ResourceUsage{CPU: info.CPU, Memory: info.Memory}
		}
		return msgs.PodMetricsMsg{Context: kubeContext, Usage: usage}
	}
}

// LoadNodeMetricsCmd is LoadPodMetricsCmd's Nodes counterpart — Nodes are
// cluster-scoped, so there's no namespace argument.
func LoadNodeMetricsCmd(ctx context.Context, client *k8s.Client, kubeContext string) tea.Cmd {
	return func() tea.Msg {
		infos, err := client.ListNodeMetrics(ctx, kubeContext)
		if err != nil {
			return msgs.NodeMetricsMsg{Context: kubeContext, Err: err}
		}
		usage := make(map[string]msgs.ResourceUsage, len(infos))
		for _, info := range infos {
			usage[info.Name] = msgs.ResourceUsage{CPU: info.CPU, Memory: info.Memory}
		}
		return msgs.NodeMetricsMsg{Context: kubeContext, Usage: usage}
	}
}

// CheckNodesAccessCmd checks whether the current credentials can watch Nodes
// in a context (see k8s.Client.CanWatchNodes), before MainPage decides
// whether to open that context's Nodes watch at all. Dispatched once per
// newly-selected context, alongside LoadNamespacesCmd.
func CheckNodesAccessCmd(ctx context.Context, client *k8s.Client, kubeContext string) tea.Cmd {
	return func() tea.Msg {
		allowed, err := client.CanWatchNodes(ctx, kubeContext)
		return msgs.NodesAccessMsg{Context: kubeContext, Allowed: allowed, Err: err}
	}
}

// LoadNamespacesCmd fetches every namespace available in a context, for the
// Namespaces pane. Dispatched once per newly-selected context.
func LoadNamespacesCmd(ctx context.Context, client *k8s.Client, kubeContext string) tea.Cmd {
	return func() tea.Msg {
		namespaces, err := client.ListNamespaces(ctx, kubeContext)
		if err != nil {
			return msgs.NamespacesMsg{Context: kubeContext, Err: err}
		}
		return msgs.NamespacesMsg{Context: kubeContext, Namespaces: namespaces}
	}
}

// LoadDeploymentDetailCmd fetches detailed information for a single deployment
func LoadDeploymentDetailCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace, deploymentName string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetDeploymentDetail(ctx, kubeContext, namespace, deploymentName)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadWorkloadPodsCmd resolves the Pods owned by a workload row (Deployment/
// StatefulSet/DaemonSet/Job/CronJob — see kinds.ResourceKind.HasPods and
// k8s.Client.ResolveWorkloadPods), for MainPage's aggregate-log "l" key on a
// non-Pods tab.
func LoadWorkloadPodsCmd(ctx context.Context, client *k8s.Client, kind msgs.ResourceKind, kubeContext, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		names, err := client.ResolveWorkloadPods(ctx, kubeContext, namespace, kind, name)
		if err != nil {
			return msgs.WorkloadPodsMsg{Context: kubeContext, Namespace: namespace, Name: name, Kind: kind, Err: err}
		}
		return msgs.WorkloadPodsMsg{Context: kubeContext, Namespace: namespace, Name: name, Kind: kind, PodNames: names}
	}
}

// LoadPodDetailCmd fetches detailed information for a single pod
func LoadPodDetailCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace, podName string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetPodDetail(ctx, kubeContext, namespace, podName)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadServiceDetailCmd fetches detailed information for a single service
func LoadServiceDetailCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace, serviceName string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetServiceDetail(ctx, kubeContext, namespace, serviceName)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadConfigMapDetailCmd fetches detailed information for a single ConfigMap
func LoadConfigMapDetailCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetConfigMapDetail(ctx, kubeContext, namespace, name)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadSecretDetailCmd fetches detailed information for a single Secret. The
// returned detail is already redacted — see k8s.GetSecretDetail.
func LoadSecretDetailCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetSecretDetail(ctx, kubeContext, namespace, name)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadJobDetailCmd fetches detailed information for a single Job
func LoadJobDetailCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetJobDetail(ctx, kubeContext, namespace, name)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadCronJobDetailCmd fetches detailed information for a single CronJob
func LoadCronJobDetailCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetCronJobDetail(ctx, kubeContext, namespace, name)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadStatefulSetDetailCmd fetches detailed information for a single StatefulSet
func LoadStatefulSetDetailCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetStatefulSetDetail(ctx, kubeContext, namespace, name)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadDaemonSetDetailCmd fetches detailed information for a single DaemonSet
func LoadDaemonSetDetailCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetDaemonSetDetail(ctx, kubeContext, namespace, name)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadIngressDetailCmd fetches detailed information for a single Ingress
func LoadIngressDetailCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetIngressDetail(ctx, kubeContext, namespace, name)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadPodDisruptionBudgetDetailCmd fetches detailed information for a single
// PodDisruptionBudget.
func LoadPodDisruptionBudgetDetailCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetPodDisruptionBudgetDetail(ctx, kubeContext, namespace, name)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadHorizontalPodAutoscalerDetailCmd fetches detailed information for a
// single HorizontalPodAutoscaler.
func LoadHorizontalPodAutoscalerDetailCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetHorizontalPodAutoscalerDetail(ctx, kubeContext, namespace, name)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadNodeDetailCmd fetches detailed information for a single Node.
// namespace is ignored — Nodes are cluster-scoped.
func LoadNodeDetailCmd(ctx context.Context, client *k8s.Client, kubeContext, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetNodeDetail(ctx, kubeContext, namespace, name)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}
