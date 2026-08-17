package watch

import (
	"github.com/ktails/ktails/internal/tui/msgs"
)

// kindSpecs maps each resource kind to its cache constructor. Watch/List
// dispatch itself no longer needs a per-kind adapter here — Cluster.Watch/
// Cluster.List (see supervisor.go) take the kind directly, and
// internal/k8s/registry.go owns the kind -> Watch<Kind>/List<Kind> fan-out
// on the production side. What's left here — building the right generic
// resourceCache[T] instantiation per kind — can't be kind-erased the same
// way, since T varies per kind at compile time. Adding a 14th kind means
// adding one entry here (plus its own row builder and Cluster-side
// registry.go entry).
var kindSpecs = map[msgs.ResourceKind]func() rowCache{
	msgs.KindPods:                     func() rowCache { return newResourceCache(podRow) },
	msgs.KindDeployments:              func() rowCache { return newResourceCache(deploymentRow) },
	msgs.KindServices:                 func() rowCache { return newResourceCache(serviceRow) },
	msgs.KindConfigMaps:               func() rowCache { return newTrimmedCache(configMapRow, trimConfigMap) },
	msgs.KindSecrets:                  func() rowCache { return newTrimmedCache(secretRow, trimSecret) },
	msgs.KindJobs:                     func() rowCache { return newResourceCache(jobRow) },
	msgs.KindCronJobs:                 func() rowCache { return newResourceCache(cronJobRow) },
	msgs.KindStatefulSets:             func() rowCache { return newResourceCache(statefulSetRow) },
	msgs.KindDaemonSets:               func() rowCache { return newResourceCache(daemonSetRow) },
	msgs.KindIngresses:                func() rowCache { return newResourceCache(ingressRow) },
	msgs.KindPodDisruptionBudgets:     func() rowCache { return newResourceCache(pdbRow) },
	msgs.KindHorizontalPodAutoscalers: func() rowCache { return newResourceCache(hpaRow) },
	msgs.KindNodes:                    func() rowCache { return newResourceCache(nodeRow) },
}
