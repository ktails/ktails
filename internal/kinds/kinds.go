// Package kinds identifies the watched Kubernetes resource types. It is a
// leaf package (no internal imports) so both internal/k8s and
// internal/tui/msgs can depend on it without a cycle — k8s.Client's Watch/
// List methods key their kind registry by ResourceKind (see
// internal/k8s/registry.go), and msgs.ResourceKind is a type alias to this
// package's ResourceKind so every existing msgs.ResourceKind/msgs.KindXxx
// reference across the codebase keeps working unchanged.
package kinds

// ResourceKind identifies one of the watched resource types. It is the
// single tab/watch/cache identity used everywhere a "Deployments" | "Pods" |
// "svc" string switch used to live.
type ResourceKind int

const (
	KindDeployments ResourceKind = iota
	KindPods
	KindServices
	KindConfigMaps
	KindSecrets
	KindJobs
	KindCronJobs
	KindStatefulSets
	KindDaemonSets
	KindIngresses
	KindPodDisruptionBudgets
	KindHorizontalPodAutoscalers
	// KindNodes is cluster-scoped, not namespaced — its rows carry no
	// Namespace, and its watch ignores the namespace argument.
	KindNodes
)

// Kinds returns every ResourceKind in tab order.
func Kinds() []ResourceKind {
	return []ResourceKind{
		KindDeployments, KindPods, KindServices, KindConfigMaps, KindSecrets,
		KindJobs, KindCronJobs, KindStatefulSets, KindDaemonSets, KindIngresses,
		KindPodDisruptionBudgets, KindHorizontalPodAutoscalers,
		KindNodes,
	}
}

// Title is the tab label shown in the Tab Area header.
func (k ResourceKind) Title() string {
	switch k {
	case KindDeployments:
		return "Deployments"
	case KindPods:
		return "Pods"
	case KindServices:
		return "Services"
	case KindConfigMaps:
		return "ConfigMaps"
	case KindSecrets:
		return "Secrets"
	case KindJobs:
		return "Jobs"
	case KindCronJobs:
		return "CronJobs"
	case KindStatefulSets:
		return "StatefulSets"
	case KindDaemonSets:
		return "DaemonSets"
	case KindIngresses:
		return "Ingresses"
	case KindPodDisruptionBudgets:
		return "PDBs"
	case KindHorizontalPodAutoscalers:
		return "HPAs"
	case KindNodes:
		return "Nodes"
	}
	return ""
}

// Kind is the Kubernetes kind name, as shown in the Detail Pane.
func (k ResourceKind) Kind() string {
	switch k {
	case KindDeployments:
		return "Deployment"
	case KindPods:
		return "Pod"
	case KindServices:
		return "Service"
	case KindConfigMaps:
		return "ConfigMap"
	case KindSecrets:
		return "Secret"
	case KindJobs:
		return "Job"
	case KindCronJobs:
		return "CronJob"
	case KindStatefulSets:
		return "StatefulSet"
	case KindDaemonSets:
		return "DaemonSet"
	case KindIngresses:
		return "Ingress"
	case KindPodDisruptionBudgets:
		return "PodDisruptionBudget"
	case KindHorizontalPodAutoscalers:
		return "HorizontalPodAutoscaler"
	case KindNodes:
		return "Node"
	}
	return ""
}

func (k ResourceKind) String() string {
	return k.Title()
}
