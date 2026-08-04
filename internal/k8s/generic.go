package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// listerWatcher is the shape every namespaced (or cluster-scoped) typed
// client interface this package uses already has — corev1client.
// ConfigMapInterface, appsv1client.DeploymentInterface, and so on. L is the
// concrete typed list a List() call returns (e.g. *corev1.ConfigMapList).
// Interface satisfaction in Go is structural, so every such typed client
// (which has these two methods plus several others this package doesn't
// need) already satisfies this without any explicit declaration.
type listerWatcher[L any] interface {
	List(ctx context.Context, opts metav1.ListOptions) (L, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
}

// pointers converts a []T slice — the shape of every *XxxList.Items field
// — to []*T, the pointer-slice shape every List<Kind> function in this
// package returns.
func pointers[T any](items []T) []*T {
	out := make([]*T, len(items))
	for i := range items {
		out[i] = &items[i]
	}
	return out
}

// watchResource is the shared body behind every Watch<Kind> function: get a
// client for kubeContext, then open a watch via getLW(clientset) — already
// bound to the right namespace (or cluster scope) by the caller. subject is
// the error message's kind phrase — "configmaps in namespace foo" for a
// namespaced kind, or just "nodes" for a cluster-scoped one — kept as a
// caller-supplied string rather than assembled here so every kind's error
// text stays byte-identical to what it was before this extraction. No
// timeout — watches must stay un-deadlined, a deadline would kill them
// mid-stream; see requestTimeout's doc comment.
func watchResource[L any](ctx context.Context, c *Client, kubeContext, subject string, getLW func(kubernetes.Interface) listerWatcher[L]) (watch.Interface, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}
	w, err := getLW(clientset).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to watch %s (context %s): %w", subject, kubeContext, err)
	}
	return w, nil
}

// listResource is the shared body behind every List<Kind> function:
// timeout-bounded client fetch + List() call, converting the typed list's
// Items via toItems (typically pointers(l.Items)) into a pointer slice. See
// watchResource's doc comment for what subject is and why it's a
// caller-supplied string rather than assembled here.
func listResource[T any, L any](ctx context.Context, c *Client, kubeContext, subject string, getLW func(kubernetes.Interface) listerWatcher[L], toItems func(L) []T) ([]T, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}
	list, err := getLW(clientset).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list %s (context %s): %w", subject, kubeContext, err)
	}
	return toItems(list), nil
}
