package k8s

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

// requestTimeout bounds every one-shot (non-watch, non-log-stream) API call
// — a hung-but-connected apiserver otherwise blocks a List/Get/metrics call
// forever, since rest.Config.Timeout is 0 (unset) and none of these calls
// used to carry a deadline of their own. Watches and log streams must stay
// un-deadlined — a deadline would kill them mid-stream — so they're
// deliberately excluded; see WatchPods and StreamLogs. A var, not a const,
// so tests can shrink it to exercise the timeout path without a real
// 15-second wait.
var requestTimeout = 15 * time.Second

// Client wraps Kubernetes client operations with support for multiple contexts
type Client struct {
	clientsByContext map[string]kubernetes.Interface
	// metricsClientsByContext is lazily populated the same way as
	// clientsByContext, but never eagerly (unlike NewClient's current-context
	// pre-create): a cluster without metrics-server installed would fail that
	// pre-create for a feature most users of a given context may never open.
	metricsClientsByContext map[string]metricsclientset.Interface
	rawConfig               *api.Config
	kubeconfigPath          string
	currentContext          string
	mu                      sync.RWMutex // Protect concurrent access

	// building/metricsBuilding track in-flight dials, per context, so a
	// second caller for the same context waits on the first's result instead
	// of dialing again — see getOrBuild. Callers for a *different* context
	// are unaffected either way, which is the fix for GetClientForContext
	// previously holding the write lock across the whole network dial (every
	// context's lookup, plus UI-thread readers like ListContexts, queued
	// behind one unreachable cluster's dial timeout).
	building        map[string]*buildResult[kubernetes.Interface]
	metricsBuilding map[string]*buildResult[metricsclientset.Interface]

	// buildClient/buildMetricsClient perform the actual dial for a context.
	// Production always wires these to createClientForContext/
	// createMetricsClientForContext (see NewClient); tests override them to
	// control dial timing without a real kubeconfig.
	buildClient        func(contextName string) (kubernetes.Interface, error)
	buildMetricsClient func(contextName string) (metricsclientset.Interface, error)
}

// buildResult is one in-flight or completed dial, shared by every caller
// requesting the same context concurrently — see getOrBuild.
type buildResult[T any] struct {
	done   chan struct{}
	client T
	err    error
}

// getOrBuild returns the cached client for name, or coordinates a single
// in-flight build via building: the first caller for a name runs build and
// populates cache; concurrent callers for the *same* name wait on that
// build's result instead of dialing again, while callers for a different
// name proceed immediately (only building[name] is touched, never a
// lock held across build()). A failed build is not cached — building[name]
// is deleted either way, so a later call retries the dial rather than
// replaying the same error forever.
func getOrBuild[T any](mu *sync.RWMutex, cache map[string]T, building map[string]*buildResult[T], name string, build func() (T, error)) (T, error) {
	mu.RLock()
	if client, ok := cache[name]; ok {
		mu.RUnlock()
		return client, nil
	}
	mu.RUnlock()

	mu.Lock()
	if client, ok := cache[name]; ok {
		mu.Unlock()
		return client, nil
	}
	if entry, ok := building[name]; ok {
		mu.Unlock()
		<-entry.done
		return entry.client, entry.err
	}
	entry := &buildResult[T]{done: make(chan struct{})}
	building[name] = entry
	mu.Unlock()

	client, err := build()

	mu.Lock()
	if err == nil {
		cache[name] = client
	}
	delete(building, name)
	entry.client, entry.err = client, err
	close(entry.done)
	mu.Unlock()

	return client, err
}

// PodInfo contains pod metadata
type PodInfo struct {
	Name            string
	Namespace       string
	Status          string
	Restarts        int32
	Age             string
	Image           string
	Container       string
	Containers      []string
	Node            string
	NodeIP          string
	PodIP           string
	ReadyContainers string // e.g. "2/3", ready vs total container statuses
	Context         string
}

type ContextsInfo struct {
	Name             string
	Cluster          string
	DefaultNamespace string
}

// getDefaultKubeconfigPath returns the default kubeconfig path
func getDefaultKubeconfigPath() string {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return kubeconfig
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kube", "config")
}

// NewClient creates a new K8s client
func NewClient(kubeconfigPath string) (*Client, error) {
	if kubeconfigPath == "" {
		kubeconfigPath = getDefaultKubeconfigPath()
		if kubeconfigPath == "" {
			return nil, fmt.Errorf("kubeconfig path is empty and could not determine default path")
		}
	}

	if _, err := os.Stat(kubeconfigPath); err != nil {
		return nil, fmt.Errorf("kubeconfig not accessible at %s: %w", kubeconfigPath, err)
	}

	// Load raw config for context/namespace operations
	loadingRules := &clientcmd.ClientConfigLoadingRules{
		ExplicitPath: kubeconfigPath,
	}
	rawConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		&clientcmd.ConfigOverrides{},
	).RawConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	currentContext := rawConfig.CurrentContext
	if currentContext == "" {
		return nil, fmt.Errorf("no current context set in kubeconfig")
	}

	client := &Client{
		clientsByContext:        make(map[string]kubernetes.Interface),
		metricsClientsByContext: make(map[string]metricsclientset.Interface),
		building:                make(map[string]*buildResult[kubernetes.Interface]),
		metricsBuilding:         make(map[string]*buildResult[metricsclientset.Interface]),
		rawConfig:               &rawConfig,
		kubeconfigPath:          kubeconfigPath,
		currentContext:          currentContext,
	}
	client.buildClient = client.createClientForContext
	client.buildMetricsClient = client.createMetricsClientForContext

	// Pre-create client for current context and test connection
	if _, err := client.GetClientForContext(currentContext); err != nil {
		return nil, fmt.Errorf("failed to connect to current context %s: %w", currentContext, err)
	}

	return client, nil
}

// buildRestConfigForContext builds the *rest.Config for one kubeconfig
// context — the shared first step behind both createClientForContext and
// createMetricsClientForContext, since a metrics.k8s.io client talks to the
// same apiserver (metrics-server is registered as an aggregated API) and
// needs the identical connection config, not a separate one.
func (c *Client) buildRestConfigForContext(contextName string) (*rest.Config, error) {
	// Check if context exists in config
	if _, exists := c.rawConfig.Contexts[contextName]; !exists {
		return nil, fmt.Errorf("context %s not found in kubeconfig", contextName)
	}

	// Create config with specific context
	loadingRules := &clientcmd.ClientConfigLoadingRules{
		ExplicitPath: c.kubeconfigPath,
	}
	configOverrides := &clientcmd.ConfigOverrides{
		CurrentContext: contextName,
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		configOverrides,
	)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create client config for context %s: %w", contextName, err)
	}
	return restConfig, nil
}

// createClientForContext creates a new clientset for the specified context
func (c *Client) createClientForContext(contextName string) (kubernetes.Interface, error) {
	restConfig, err := c.buildRestConfigForContext(contextName)
	if err != nil {
		return nil, err
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client for context %s: %w", contextName, err)
	}

	// Test the connection. ServerVersion() takes no context (and so cannot
	// be bounded by one), which is why the previous version of this probe's
	// context.WithTimeout was silently discarded and did nothing — probe via
	// the REST client directly instead, which does honor ctx.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := clientset.Discovery().RESTClient().Get().AbsPath("/version").Do(ctx).Error(); err != nil {
		return nil, fmt.Errorf("failed to connect to cluster in context %s: %w", contextName, err)
	}

	return clientset, nil
}

// createMetricsClientForContext creates a new metrics.k8s.io clientset for
// the specified context, reusing the same rest.Config as the regular
// clientset. Unlike createClientForContext, it doesn't test the connection
// eagerly — metrics-server is an optional, commonly-absent aggregated API,
// so callers (ListPodMetrics/ListNodeMetrics) are expected to handle a
// failing call, not a failing client construction.
func (c *Client) createMetricsClientForContext(contextName string) (metricsclientset.Interface, error) {
	restConfig, err := c.buildRestConfigForContext(contextName)
	if err != nil {
		return nil, err
	}

	clientset, err := metricsclientset.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics client for context %s: %w", contextName, err)
	}
	return clientset, nil
}

// GetMetricsClientForContext returns a metrics.k8s.io clientset for the
// specified context, creating and caching it if it doesn't exist yet — the
// same lazy-cache shape as GetClientForContext, including the same
// single-flight dial coordination (see getOrBuild).
func (c *Client) GetMetricsClientForContext(contextName string) (metricsclientset.Interface, error) {
	c.mu.Lock()
	if c.metricsBuilding == nil {
		c.metricsBuilding = make(map[string]*buildResult[metricsclientset.Interface])
	}
	build := c.buildMetricsClient
	if build == nil {
		build = c.createMetricsClientForContext
	}
	c.mu.Unlock()

	return getOrBuild(&c.mu, c.metricsClientsByContext, c.metricsBuilding, contextName, func() (metricsclientset.Interface, error) {
		return build(contextName)
	})
}

// GetClientForContext returns a clientset for the specified context,
// creating and caching it if it doesn't exist yet. Concurrent requests for
// the *same* context share one dial and wait on its result together;
// requests for a *different* context never block on it — see getOrBuild.
// Previously this held the write lock across the whole network dial
// (createClientForContext's ServerVersion() round trip has no deadline), so
// one unreachable cluster froze every other context's lookup, including
// UI-thread readers like ListContexts/GetCurrentContext/DefaultNamespace.
func (c *Client) GetClientForContext(contextName string) (kubernetes.Interface, error) {
	c.mu.Lock()
	if c.building == nil {
		c.building = make(map[string]*buildResult[kubernetes.Interface])
	}
	build := c.buildClient
	if build == nil {
		build = c.createClientForContext
	}
	c.mu.Unlock()

	return getOrBuild(&c.mu, c.clientsByContext, c.building, contextName, func() (kubernetes.Interface, error) {
		return build(contextName)
	})
}

// GetCurrentContext returns the currently active context
func (c *Client) GetCurrentContext() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentContext
}

// DefaultNamespace returns the default namespace for the specified context
func (c *Client) DefaultNamespace(kubeContext string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if ctx, exists := c.rawConfig.Contexts[kubeContext]; exists {
		if ctx.Namespace != "" {
			return ctx.Namespace
		}
	}
	return "default"
}

// ListContexts returns available contexts from kubeconfig
func (c *Client) ListContexts() ([]ContextsInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	contexts := make([]ContextsInfo, 0, len(c.rawConfig.Contexts))
	for name, value := range c.rawConfig.Contexts {
		ctx := ContextsInfo{
			Name:             name,
			Cluster:          value.Cluster,
			DefaultNamespace: value.Namespace,
		}
		contexts = append(contexts, ctx)
	}
	return contexts, nil
}

// CanWatchNodes reports whether the current credentials can watch Nodes in
// the given context, via a SelfSubjectAccessReview — the standard
// "can I do X" k8s API that answers without actually attempting the watch.
// Nodes is cluster-scoped and commonly restricted to admins, so callers use
// this to skip opening a watch that would just fail, rather than reacting
// to the failure after the fact (see watch.Supervisor's Forbidden handling
// for kinds this check doesn't cover).
func (c *Client) CanWatchNodes(kubeContext string) (bool, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return false, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	review := &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Verb:     "watch",
				Resource: "nodes",
			},
		},
	}
	result, err := clientset.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to check node access for context %s: %w", kubeContext, err)
	}
	return result.Status.Allowed, nil
}

// ListNamespaces returns the names of every namespace in the given context,
// sorted alphabetically — used by the Namespaces pane. A Forbidden error
// (the ServiceAccount lacks cluster-scoped "list namespaces") is rendered as
// a clear RBAC message rather than the raw client-go error, matching how
// watch.Supervisor surfaces RBAC-denied watches.
func (c *Client) ListNamespaces(kubeContext string) ([]string, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	list, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsForbidden(err) {
			return nil, fmt.Errorf("RBAC: not permitted to list namespaces in context '%s'", kubeContext)
		}
		return nil, fmt.Errorf("failed to list namespaces in context %s: %w", kubeContext, err)
	}

	names := make([]string, 0, len(list.Items))
	for _, ns := range list.Items {
		names = append(names, ns.Name)
	}
	sort.Strings(names)
	return names, nil
}

// podsLW binds a PodInterface's List/Watch to namespace, for
// watchResource/listResource — see generic.go.
func podsLW(namespace string) func(kubernetes.Interface) listerWatcher[*v1.PodList] {
	return func(cs kubernetes.Interface) listerWatcher[*v1.PodList] {
		return cs.CoreV1().Pods(namespace)
	}
}

// WatchPods opens a watch on pods in the given namespace. A bare Watch with
// no ResourceVersion set has the server replay every currently-existing
// object as a synthetic Added event before continuing with live changes, so
// no separate initial List() call is needed to keep the watch itself
// correct — ListPods exists purely so callers can paint a fast, atomic
// initial table instead of watching that replay trickle in one event at a
// time (see watch.Supervisor.start).
func (c *Client) WatchPods(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	return watchResource(ctx, c, kubeContext, "pods in namespace "+namespace, podsLW(namespace))
}

// ListPods fetches every pod in the given namespace in one call. See
// WatchPods for why this exists alongside the watch.
func (c *Client) ListPods(ctx context.Context, kubeContext, namespace string) ([]*v1.Pod, error) {
	return listResource(ctx, c, kubeContext, "pods in namespace "+namespace, podsLW(namespace),
		func(l *v1.PodList) []*v1.Pod { return pointers(l.Items) })
}

// deploymentsLW binds a DeploymentInterface's List/Watch to namespace, for
// watchResource/listResource — see generic.go.
func deploymentsLW(namespace string) func(kubernetes.Interface) listerWatcher[*appsv1.DeploymentList] {
	return func(cs kubernetes.Interface) listerWatcher[*appsv1.DeploymentList] {
		return cs.AppsV1().Deployments(namespace)
	}
}

// WatchDeployments opens a watch on deployments in the given namespace. See
// WatchPods for the implicit list-then-watch behavior.
func (c *Client) WatchDeployments(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	return watchResource(ctx, c, kubeContext, "deployments in namespace "+namespace, deploymentsLW(namespace))
}

// ListDeployments fetches every deployment in the given namespace in one
// call. See ListPods for why this exists alongside the watch.
func (c *Client) ListDeployments(ctx context.Context, kubeContext, namespace string) ([]*appsv1.Deployment, error) {
	return listResource(ctx, c, kubeContext, "deployments in namespace "+namespace, deploymentsLW(namespace),
		func(l *appsv1.DeploymentList) []*appsv1.Deployment { return pointers(l.Items) })
}

// servicesLW binds a ServiceInterface's List/Watch to namespace, for
// watchResource/listResource — see generic.go.
func servicesLW(namespace string) func(kubernetes.Interface) listerWatcher[*v1.ServiceList] {
	return func(cs kubernetes.Interface) listerWatcher[*v1.ServiceList] {
		return cs.CoreV1().Services(namespace)
	}
}

// WatchServices opens a watch on services in the given namespace. See
// WatchPods for the implicit list-then-watch behavior.
func (c *Client) WatchServices(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	return watchResource(ctx, c, kubeContext, "services in namespace "+namespace, servicesLW(namespace))
}

// ListServices fetches every service in the given namespace in one call. See
// ListPods for why this exists alongside the watch.
func (c *Client) ListServices(ctx context.Context, kubeContext, namespace string) ([]*v1.Service, error) {
	return listResource(ctx, c, kubeContext, "services in namespace "+namespace, servicesLW(namespace),
		func(l *v1.ServiceList) []*v1.Service { return pointers(l.Items) })
}

// GetPodDetail fetches a single pod's status, rendered YAML, and recent events.
func (c *Client) GetPodDetail(kubeContext, namespace, podName string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "Pod"}
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get pod %s in namespace %s (context %s): %w", podName, namespace, kubeContext, err)
	}

	var restarts int32
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
	}

	d.Name = pod.Name
	d.Namespace = pod.Namespace
	d.Age = formatDuration(time.Since(pod.CreationTimestamp.Time))
	d.Summary = fmt.Sprintf("Phase: %s  Restarts: %d  Node: %s", pod.Status.Phase, restarts, pod.Spec.NodeName)
	for _, condition := range pod.Status.Conditions {
		d.Status = append(d.Status, formatCondition(string(condition.Type), string(condition.Status), condition.Reason, condition.Message))
	}

	if len(pod.Status.ContainerStatuses) > 0 {
		d.Status = append(d.Status, "", "Containers:")
		for _, cs := range pod.Status.ContainerStatuses {
			d.Status = append(d.Status, fmt.Sprintf("  %s: %s  ready=%t restarts=%d image=%s",
				cs.Name, containerStateString(cs.State), cs.Ready, cs.RestartCount, cs.Image))
		}
	}

	d.YAML = renderDetailYAML(pod, "v1", "Pod")

	if events, err := c.getEvents(kubeContext, namespace, "Pod", podName); err == nil {
		d.Events = events
	}

	return d, nil
}

// PodToPodInfo converts a pod object to PodInfo
func PodToPodInfo(pod *v1.Pod, kubeContext string) *PodInfo {
	age := time.Since(pod.CreationTimestamp.Time)

	var restarts int32
	for _, containerStatus := range pod.Status.ContainerStatuses {
		restarts += containerStatus.RestartCount
	}

	var image, container string
	containers := make([]string, 0, len(pod.Spec.Containers))
	if len(pod.Spec.Containers) > 0 {
		container = pod.Spec.Containers[0].Name
		image = pod.Spec.Containers[0].Image
	}
	for _, c := range pod.Spec.Containers {
		containers = append(containers, c.Name)
	}

	var readyCount int
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			readyCount++
		}
	}
	readyContainers := fmt.Sprintf("%d/%d", readyCount, len(pod.Status.ContainerStatuses))

	return &PodInfo{
		Name:            pod.Name,
		Namespace:       pod.Namespace,
		Context:         kubeContext,
		Status:          string(pod.Status.Phase),
		Restarts:        restarts,
		Age:             formatDuration(age),
		Image:           image,
		Container:       container,
		Containers:      containers,
		Node:            pod.Spec.NodeName,
		NodeIP:          pod.Status.HostIP,
		PodIP:           pod.Status.PodIP,
		ReadyContainers: readyContainers,
	}
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
}

// formatCondition renders a status condition, omitting the reason/message
// parenthetical when both are empty (common for healthy conditions).
func formatCondition(condType, status, reason, message string) string {
	if reason == "" && message == "" {
		return fmt.Sprintf("%s=%s", condType, status)
	}
	return fmt.Sprintf("%s=%s (%s: %s)", condType, status, reason, message)
}

// containerStateString summarizes a container's current state for display.
func containerStateString(state v1.ContainerState) string {
	switch {
	case state.Running != nil:
		return fmt.Sprintf("Running (started %s ago)", formatDuration(time.Since(state.Running.StartedAt.Time)))
	case state.Waiting != nil:
		if state.Waiting.Message != "" {
			return fmt.Sprintf("Waiting (%s: %s)", state.Waiting.Reason, state.Waiting.Message)
		}
		return fmt.Sprintf("Waiting (%s)", state.Waiting.Reason)
	case state.Terminated != nil:
		return fmt.Sprintf("Terminated (%s, exit %d)", state.Terminated.Reason, state.Terminated.ExitCode)
	default:
		return "Unknown"
	}
}

// StreamLogs streams logs from a pod
func (c *Client) StreamLogs(kubeContext, namespace, podName string, opts *v1.PodLogOptions) (io.ReadCloser, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	if opts == nil {
		opts = &v1.PodLogOptions{
			Follow: true,
		}
	}

	ctx := context.Background()
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to stream logs from pod %s: %w", podName, err)
	}

	return stream, nil
}
