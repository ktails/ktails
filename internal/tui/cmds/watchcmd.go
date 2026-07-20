package cmds

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
)

// WatchPodsCmd opens a Pods watch for one context+namespace. generation is
// echoed back on the resulting message so the caller can tell whether this
// watch is still the one it's waiting for (it may have been superseded by a
// manual "r" restart or a context deselect before this resolves).
func WatchPodsCmd(client *k8s.Client, kubeContext, namespace string, generation int) tea.Cmd {
	return func() tea.Msg {
		w, err := client.WatchPods(context.Background(), kubeContext, namespace)
		if err != nil {
			return msgs.PodWatchClosedMsg{Context: kubeContext, Generation: generation, Err: err}
		}
		return msgs.PodWatchOpenedMsg{Context: kubeContext, Generation: generation, Watcher: w}
	}
}

// WaitForPodWatchEventCmd blocks for the next event on watcher, applies it
// to cache, then non-blockingly drains any additional already-buffered
// events (collapsing bursts — e.g. the initial full-list replay, or a mass
// rollout — into fewer UI updates instead of one message per object) before
// returning a freshly rebuilt row set. The caller re-issues this command
// after each PodWatchEventMsg to keep the read loop going.
func WaitForPodWatchEventCmd(kubeContext string, generation int, watcher watch.Interface, cache *PodWatchCache) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-watcher.ResultChan()
		if !ok {
			return msgs.PodWatchClosedMsg{Context: kubeContext, Generation: generation}
		}
		if err := cache.apply(ev); err != nil {
			return msgs.PodWatchClosedMsg{Context: kubeContext, Generation: generation, Err: err}
		}

		for {
			select {
			case ev, ok := <-watcher.ResultChan():
				if !ok {
					return msgs.PodWatchEventMsg{Context: kubeContext, Generation: generation, Rows: cache.Rows(kubeContext)}
				}
				if err := cache.apply(ev); err != nil {
					return msgs.PodWatchClosedMsg{Context: kubeContext, Generation: generation, Err: err}
				}
			default:
				return msgs.PodWatchEventMsg{Context: kubeContext, Generation: generation, Rows: cache.Rows(kubeContext)}
			}
		}
	}
}

// ReconnectPodsCmd sleeps for delay (exponential backoff between watch
// reconnect attempts), then opens a fresh Pods watch exactly like
// WatchPodsCmd. The existing cache is reused as-is — a fresh watch's Added
// replay is idempotent against the upsert-based apply.
func ReconnectPodsCmd(client *k8s.Client, kubeContext, namespace string, generation int, delay time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(delay)
		w, err := client.WatchPods(context.Background(), kubeContext, namespace)
		if err != nil {
			return msgs.PodWatchClosedMsg{Context: kubeContext, Generation: generation, Err: err}
		}
		return msgs.PodWatchOpenedMsg{Context: kubeContext, Generation: generation, Watcher: w}
	}
}

// WatchDeploymentsCmd mirrors WatchPodsCmd for Deployments.
func WatchDeploymentsCmd(client *k8s.Client, kubeContext, namespace string, generation int) tea.Cmd {
	return func() tea.Msg {
		w, err := client.WatchDeployments(context.Background(), kubeContext, namespace)
		if err != nil {
			return msgs.DeploymentWatchClosedMsg{Context: kubeContext, Generation: generation, Err: err}
		}
		return msgs.DeploymentWatchOpenedMsg{Context: kubeContext, Generation: generation, Watcher: w}
	}
}

// WaitForDeploymentWatchEventCmd mirrors WaitForPodWatchEventCmd for Deployments.
func WaitForDeploymentWatchEventCmd(kubeContext string, generation int, watcher watch.Interface, cache *DeploymentWatchCache) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-watcher.ResultChan()
		if !ok {
			return msgs.DeploymentWatchClosedMsg{Context: kubeContext, Generation: generation}
		}
		if err := cache.apply(ev); err != nil {
			return msgs.DeploymentWatchClosedMsg{Context: kubeContext, Generation: generation, Err: err}
		}

		for {
			select {
			case ev, ok := <-watcher.ResultChan():
				if !ok {
					return msgs.DeploymentWatchEventMsg{Context: kubeContext, Generation: generation, Rows: cache.Rows(kubeContext)}
				}
				if err := cache.apply(ev); err != nil {
					return msgs.DeploymentWatchClosedMsg{Context: kubeContext, Generation: generation, Err: err}
				}
			default:
				return msgs.DeploymentWatchEventMsg{Context: kubeContext, Generation: generation, Rows: cache.Rows(kubeContext)}
			}
		}
	}
}

// ReconnectDeploymentsCmd mirrors ReconnectPodsCmd for Deployments.
func ReconnectDeploymentsCmd(client *k8s.Client, kubeContext, namespace string, generation int, delay time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(delay)
		w, err := client.WatchDeployments(context.Background(), kubeContext, namespace)
		if err != nil {
			return msgs.DeploymentWatchClosedMsg{Context: kubeContext, Generation: generation, Err: err}
		}
		return msgs.DeploymentWatchOpenedMsg{Context: kubeContext, Generation: generation, Watcher: w}
	}
}

// WatchServicesCmd mirrors WatchPodsCmd for Services.
func WatchServicesCmd(client *k8s.Client, kubeContext, namespace string, generation int) tea.Cmd {
	return func() tea.Msg {
		w, err := client.WatchServices(context.Background(), kubeContext, namespace)
		if err != nil {
			return msgs.ServiceWatchClosedMsg{Context: kubeContext, Generation: generation, Err: err}
		}
		return msgs.ServiceWatchOpenedMsg{Context: kubeContext, Generation: generation, Watcher: w}
	}
}

// WaitForServiceWatchEventCmd mirrors WaitForPodWatchEventCmd for Services.
func WaitForServiceWatchEventCmd(kubeContext string, generation int, watcher watch.Interface, cache *ServiceWatchCache) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-watcher.ResultChan()
		if !ok {
			return msgs.ServiceWatchClosedMsg{Context: kubeContext, Generation: generation}
		}
		if err := cache.apply(ev); err != nil {
			return msgs.ServiceWatchClosedMsg{Context: kubeContext, Generation: generation, Err: err}
		}

		for {
			select {
			case ev, ok := <-watcher.ResultChan():
				if !ok {
					return msgs.ServiceWatchEventMsg{Context: kubeContext, Generation: generation, Rows: cache.Rows(kubeContext)}
				}
				if err := cache.apply(ev); err != nil {
					return msgs.ServiceWatchClosedMsg{Context: kubeContext, Generation: generation, Err: err}
				}
			default:
				return msgs.ServiceWatchEventMsg{Context: kubeContext, Generation: generation, Rows: cache.Rows(kubeContext)}
			}
		}
	}
}

// ReconnectServicesCmd mirrors ReconnectPodsCmd for Services.
func ReconnectServicesCmd(client *k8s.Client, kubeContext, namespace string, generation int, delay time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(delay)
		w, err := client.WatchServices(context.Background(), kubeContext, namespace)
		if err != nil {
			return msgs.ServiceWatchClosedMsg{Context: kubeContext, Generation: generation, Err: err}
		}
		return msgs.ServiceWatchOpenedMsg{Context: kubeContext, Generation: generation, Watcher: w}
	}
}
