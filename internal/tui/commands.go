package tui

import (
	"bufio"
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivyascorp-net/ktails/internal/k8s"
	v1 "k8s.io/api/core/v1"
)

// Global reference to K8s client (set in model)
var globalK8sClient *k8s.Client

// Active stream channels per pane
var activeStreamChans = map[int]chan tea.Msg{}

// SetK8sClient sets the global K8s client for commands
func SetK8sClient(client *k8s.Client) {
	globalK8sClient = client
}

// loadContextsCmd loads available Kubernetes contexts
func loadContextsCmd(client *k8s.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return ContextsLoadedMsg{
				Err: fmt.Errorf("K8s client not initialized"),
			}
		}

		contexts, err := client.ListContexts()
		if err != nil {
			return ContextsLoadedMsg{
				Err: err,
			}
		}

		current := client.GetCurrentContext()

		return ContextsLoadedMsg{
			Contexts: contexts,
			Current:  current,
			Err:      nil,
		}
	}
}

// loadNamespacesCmd loads namespaces for a context
func loadNamespacesCmd(kubeContexts map[string]*k8s.Client) tea.Cmd {
	namespacesMsg := []NamespacesLoadedMsg{}
	for contextName, client := range kubeContexts {
		
		n, err := client.ListNamespaces(contextName)

		namespacesMsg = append(namespacesMsg, NamespacesLoadedMsg{
			Namespaces: n,
			Context:    contextName,
			Err:        err,
		})
	}
	teaCmd := func() tea.Msg {
		return namespacesMsg
	}
	return teaCmd

}

// loadNamespacesForSingleCmd loads namespaces for a single context using provided client
func loadNamespacesForSingleCmd(cli *k8s.Client, contextName string) tea.Cmd {
	return func() tea.Msg {
		if cli == nil {
			return []NamespacesLoadedMsg{{Context: contextName, Err: fmt.Errorf("K8s client not provided")}}
		}
		ns, err := cli.ListNamespaces(contextName)
		return []NamespacesLoadedMsg{{Namespaces: ns, Context: contextName, Err: err}}
	}
}

// loadPodsCmd loads pods for a context and namespace using the global client
func loadPodsCmd(context, namespace string) tea.Cmd {
	return func() tea.Msg {
		if globalK8sClient == nil {
			return PodsLoadedMsg{
				Err: fmt.Errorf("K8s client not initialized"),
			}
		}

		pods, err := globalK8sClient.ListPods(context, namespace)
		if err != nil {
			return PodsLoadedMsg{
				Err: err,
			}
		}

		// Convert to PodItems
		podItems := make([]PodItem, len(pods))
		for i, pod := range pods {
			ready := getPodReadyStatus(&pod)
			podItems[i] = PodItem{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Status:    string(pod.Status.Phase),
				Ready:     ready,
				Phase:     pod.Status.Phase,
			}
		}

		return PodsLoadedMsg{
			Pods:      podItems,
			Context:   context,
			Namespace: namespace,
			Err:       nil,
		}
	}
}

// loadPodsForClientCmd loads pods for a context and namespace using the provided client
func loadPodsForClientCmd(cli *k8s.Client, context, namespace string) tea.Cmd {
	return func() tea.Msg {
		if cli == nil {
			return PodsLoadedMsg{Err: fmt.Errorf("K8s client not provided")}
		}
		pods, err := cli.ListPods(context, namespace)
		if err != nil {
			return PodsLoadedMsg{Err: err}
		}
		podItems := make([]PodItem, len(pods))
		for i, pod := range pods {
			ready := getPodReadyStatus(&pod)
			podItems[i] = PodItem{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Status:    string(pod.Status.Phase),
				Ready:     ready,
				Phase:     pod.Status.Phase,
			}
		}
		return PodsLoadedMsg{Pods: podItems, Context: context, Namespace: namespace}
	}
}

// getPodReadyStatus returns the ready status as "X/Y"
func getPodReadyStatus(pod *v1.Pod) string {
	total := len(pod.Status.ContainerStatuses)
	ready := 0
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
	}
	return fmt.Sprintf("%d/%d", ready, total)
}

// loadPodInfoCmd loads pod information
func loadPodInfoCmd(cli *k8s.Client, paneIndex int, context, namespace, podName string) tea.Cmd {
	return func() tea.Msg {

		info, err := cli.GetPodInfo(context, namespace, podName)
		if err != nil {
			return PodInfoMsg{
				PaneIndex: paneIndex,
				Err:       err,
			}
		}

		return PodInfoMsg{
			PaneIndex: paneIndex,
			Info:      info,
			Err:       nil,
		}
	}
}

// startLogStreamForClientCmd starts streaming logs for a pod using the provided client.
func startLogStreamForClientCmd(paneIndex int, cli *k8s.Client, namespace, podName, container string) tea.Cmd {
	// Create a fresh channel for this pane's stream
	ch := make(chan tea.Msg, 100)
	activeStreamChans[paneIndex] = ch

	// Start goroutine to stream logs
	go func(out chan tea.Msg) {
		if cli == nil {
			out <- ErrorMsg{PaneIndex: paneIndex, Err: fmt.Errorf("K8s client not provided")}
			close(out)
			return
		}

		logOpts := &v1.PodLogOptions{Container: container, Follow: true, Timestamps: true}
		rc, err := cli.StreamLogs(context.Background(), namespace, podName, logOpts)
		if err != nil {
			out <- ErrorMsg{PaneIndex: paneIndex, Err: err}
			close(out)
			return
		}
		defer rc.Close()

		scanner := bufio.NewScanner(rc)
		for scanner.Scan() {
			line := scanner.Text()
			out <- LogLineMsg{PaneIndex: paneIndex, Line: line, Timestamp: time.Now()}
		}
		if err := scanner.Err(); err != nil {
			out <- ErrorMsg{PaneIndex: paneIndex, Err: err}
		}
		out <- StreamEndedMsg{PaneIndex: paneIndex}
		close(out)
	}(ch)

	// Return a command that reads one message from the stream channel
	return continueLogStreamCmd(paneIndex)
}

// continueLogStreamCmd reads the next message from an existing stream channel.
func continueLogStreamCmd(paneIndex int) tea.Cmd {
	return func() tea.Msg {
		ch := activeStreamChans[paneIndex]
		if ch == nil {
			return nil
		}
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}
