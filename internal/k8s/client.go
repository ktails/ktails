// Package k8s
package k8s

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// Client wraps Kubernetes client operations
type Client struct {
	clientset      *kubernetes.Clientset
	clientConfig   clientcmd.ClientConfig
	rawConfig      *api.Config
	kubeconfigPath string
	currentContext string
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

// getDefaultKubeconfigPath returns the default kubeconfig path
func getDefaultKubeconfigPath() string {
	// Check KUBECONFIG env var first
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return kubeconfig
	}

	// Default to ~/.kube/config
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
	}
	_, err := os.Stat(kubeconfigPath)
	if os.IsNotExist(err) {
		return &Client{}, fmt.Errorf("unable to find kubeconfig %v", err)
	}
	// Load kubeconfig
	loadingRules := &clientcmd.ClientConfigLoadingRules{
		ExplicitPath: kubeconfigPath,
	}
	configOverrides := &clientcmd.ConfigOverrides{}

	// Create client config
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		configOverrides,
	)

	// Get raw config (for context/namespace operations)
	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Get current context
	currentContext := rawConfig.CurrentContext
	if currentContext == "" {
		return nil, fmt.Errorf("no current context set in kubeconfig")
	}

	// Build rest config for the current context
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create client config: %w", err)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Test the connection by getting server version
	_, err = clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to kubernetes cluster: %w", err)
	}

	return &Client{
		clientset:      clientset,
		clientConfig:   clientConfig,
		rawConfig:      &rawConfig,
		currentContext: currentContext,
		kubeconfigPath: kubeconfigPath,
	}, nil
}

// GetCurrentContext returns the currently active context
func (c *Client) GetCurrentContext() string {
	return c.currentContext
}

// DefultNamespace returns the default namespace for the current context
func (c *Client) DefaultNamespace(kubeContext string) string {
	if ctx, exists := c.rawConfig.Contexts[kubeContext]; exists {
		if ctx.Namespace != "" {
			return ctx.Namespace
		}
	}
	return "default"
}

// ListContexts returns available contexts from kubeconfig
func (c *Client) ListContexts() ([]string, error) {
	contexts := make([]string, 0, len(c.rawConfig.Contexts))
	for name := range c.rawConfig.Contexts {
		contexts = append(contexts, name)
	}
	return contexts, nil
}

// ListNamespaces returns namespaces from the specified Kubernetes context.
// If kubeContext is empty, uses the current context.
func (c *Client) ListNamespaces(kubeContext string) ([]string, error) {
	// Switch to specified context if provided and different from current
	if kubeContext != "" && kubeContext != c.currentContext {
		if err := c.SwitchContext(kubeContext); err != nil {
			return nil, fmt.Errorf("failed to switch to context %s: %w", kubeContext, err)
		}
	}

	// List namespaces
	ctx := context.Background()
	namespaceList, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
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
	if kubeContext != "" && kubeContext != c.currentContext {
		if err := c.SwitchContext(kubeContext); err != nil {
			return nil, fmt.Errorf("failed to switch to context %s: %w", kubeContext, err)
		}
	}
	// List Pods
	pList, err := c.clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list Pods in the namespace %s using context %s, err: %v", namespace, kubeContext, err)
	}

	return pList.Items, nil
}

// GetPodInfo fetches detailed pod information
func (c *Client) GetPodInfo(kubeContext, namespace, podName string) (*PodInfo, error) {
	// Switch to specified context if provided and different from current
	if kubeContext != "" && kubeContext != c.currentContext {
		if err := c.SwitchContext(kubeContext); err != nil {
			return nil, fmt.Errorf("failed to switch to context %s: %w", kubeContext, err)
		}
	}

	// Get pod
	ctx := context.Background()
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s: %w", podName, err)
	}

	// Calculate age
	age := time.Since(pod.CreationTimestamp.Time)

	// Count restarts
	var restarts int32
	for _, containerStatus := range pod.Status.ContainerStatuses {
		restarts += containerStatus.RestartCount
	}

	// Get primary container info
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
	}, nil
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
func (c *Client) StreamLogs(ctx context.Context, namespace, podName string, opts *v1.PodLogOptions) (io.ReadCloser, error) {
	// Use default options if none provided
	if opts == nil {
		opts = &v1.PodLogOptions{
			Follow: true,
		}
	}

	// Get log stream
	req := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to stream logs from pod %s: %w", podName, err)
	}

	return stream, nil
}

// SwitchContext switches the current context
func (c *Client) SwitchContext(contextName string) error {
	// Check if context exists
	if _, exists := c.rawConfig.Contexts[contextName]; !exists {
		return fmt.Errorf("context %s not found in kubeconfig", contextName)
	}

	// Create new config with overridden context
	loadingRules := &clientcmd.ClientConfigLoadingRules{
		ExplicitPath: c.kubeconfigPath,
	}
	configOverrides := &clientcmd.ConfigOverrides{
		CurrentContext: contextName,
	}

	// Create new client config
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		configOverrides,
	)

	// Build rest config
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("failed to create client config for context %s: %w", contextName, err)
	}

	// Create new clientset
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client for context %s: %w", contextName, err)
	}

	// Test the connection
	_, err = clientset.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("failed to connect to cluster in context %s: %w", contextName, err)
	}

	// Update client
	c.clientset = clientset
	c.clientConfig = clientConfig
	c.currentContext = contextName

	return nil
}
