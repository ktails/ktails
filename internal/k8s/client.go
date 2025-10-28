package k8s

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// Client wraps Kubernetes client operations with support for multiple contexts
type Client struct {
	clientsByContext map[string]*kubernetes.Clientset
	rawConfig        *api.Config
	kubeconfigPath   string
	currentContext   string
	mu               sync.RWMutex // Protect concurrent access
}

// PodInfo contains pod metadata
type PodInfo struct {
	Name      string
	Namespace string
	Status    string
	Restarts  int32
	Age       string
	Image     string
	Container string
	Node      string
	Context   string
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
		clientsByContext: make(map[string]*kubernetes.Clientset),
		rawConfig:        &rawConfig,
		kubeconfigPath:   kubeconfigPath,
		currentContext:   currentContext,
	}

	// Pre-create client for current context and test connection
	if _, err := client.GetClientForContext(currentContext); err != nil {
		return nil, fmt.Errorf("failed to connect to current context %s: %w", currentContext, err)
	}

	return client, nil
}

// createClientForContext creates a new clientset for the specified context
func (c *Client) createClientForContext(contextName string) (*kubernetes.Clientset, error) {
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

	// Build rest config for this context
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create client config for context %s: %w", contextName, err)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client for context %s: %w", contextName, err)
	}

	// Test the connection
	_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to cluster in context %s: %w", contextName, err)
	}

	return clientset, nil
}

// GetClientForContext returns a clientset for the specified context
// Creates and caches the client if it doesn't exist yet
func (c *Client) GetClientForContext(contextName string) (*kubernetes.Clientset, error) {
	// Try to get existing client with read lock
	c.mu.RLock()
	if client, exists := c.clientsByContext[contextName]; exists {
		c.mu.RUnlock()
		return client, nil
	}
	c.mu.RUnlock()

	// Create new client with write lock
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have created it)
	if client, exists := c.clientsByContext[contextName]; exists {
		return client, nil
	}

	// Create the client
	client, err := c.createClientForContext(contextName)
	if err != nil {
		return nil, err
	}

	// Cache it
	c.clientsByContext[contextName] = client
	return client, nil
}

// GetCurrentContext returns the currently active context
func (c *Client) GetCurrentContext() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentContext
}

// SetCurrentContext changes the current context (doesn't affect cached clients)
func (c *Client) SetCurrentContext(contextName string) error {
	// Verify context exists
	c.mu.RLock()
	_, exists := c.rawConfig.Contexts[contextName]
	c.mu.RUnlock()

	if !exists {
		return fmt.Errorf("context %s not found in kubeconfig", contextName)
	}

	// Ensure client exists for this context
	if _, err := c.GetClientForContext(contextName); err != nil {
		return err
	}

	c.mu.Lock()
	c.currentContext = contextName
	c.mu.Unlock()

	return nil
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

// ListNamespaces returns namespaces from the specified Kubernetes context
func (c *Client) ListNamespaces(kubeContext string) ([]string, error) {
	// Get client for this context
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	// List namespaces
	ctx := context.Background()
	namespaceList, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces in context %s: %w", kubeContext, err)
	}

	// Extract namespace names
	namespaces := make([]string, 0, len(namespaceList.Items))
	for _, ns := range namespaceList.Items {
		namespaces = append(namespaces, ns.Name)
	}

	return namespaces, nil
}

// ListPods returns pods in the given namespace
func (c *Client) ListPods(kubeContext, namespace string) ([]v1.Pod, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	pList, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s (context %s): %w", namespace, kubeContext, err)
	}

	return pList.Items, nil
}

// ListPodInfo returns pods with detailed information
func (c *Client) ListPodInfo(kubeContext, namespace string) ([]*PodInfo, error) {
	pods, err := c.ListPods(kubeContext, namespace)
	if err != nil {
		return nil, err
	}

	podInfos := make([]*PodInfo, 0, len(pods))
	for _, pod := range pods {
		info := c.podToPodInfo(&pod, kubeContext)
		podInfos = append(podInfos, info)
	}

	return podInfos, nil
}

// GetPodInfo fetches detailed pod information
func (c *Client) GetPodInfo(kubeContext, namespace, podName string) (*PodInfo, error) {
	clientset, err := c.GetClientForContext(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContext, err)
	}

	ctx := context.Background()
	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s in namespace %s (context %s): %w", podName, namespace, kubeContext, err)
	}

	return c.podToPodInfo(pod, kubeContext), nil
}

// podToPodInfo converts a pod object to PodInfo
func (c *Client) podToPodInfo(pod *v1.Pod, kubeContext string) *PodInfo {
	age := time.Since(pod.CreationTimestamp.Time)

	var restarts int32
	for _, containerStatus := range pod.Status.ContainerStatuses {
		restarts += containerStatus.RestartCount
	}

	var image, container string
	if len(pod.Spec.Containers) > 0 {
		container = pod.Spec.Containers[0].Name
		image = pod.Spec.Containers[0].Image
	}

	return &PodInfo{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Context:   kubeContext,
		Status:    string(pod.Status.Phase),
		Restarts:  restarts,
		Age:       formatDuration(age),
		Image:     image,
		Container: container,
		Node:      pod.Spec.NodeName,
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

// ClearClientCache removes cached client for a specific context
// Useful if connection needs to be refreshed
func (c *Client) ClearClientCache(contextName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.clientsByContext, contextName)
}

// ClearAllClientCaches removes all cached clients
func (c *Client) ClearAllClientCaches() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clientsByContext = make(map[string]*kubernetes.Clientset)
}
