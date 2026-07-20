// Package cmds implement interface to k8s client
package cmds

import (
	"bufio"
	"io"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	v1 "k8s.io/api/core/v1"

	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
)

// logTailLines is the number of pre-existing lines backfilled when a log
// stream opens, matching `kubectl logs -f --tail=N`.
const logTailLines = 200

// maxLogLineBytes bounds bufio.Scanner's per-line buffer so a single
// abnormally long log line (e.g. a huge JSON blob) can't abort the scan
// with bufio.ErrTooLong.
const maxLogLineBytes = 1024 * 1024

func int64Ptr(v int64) *int64 {
	return &v
}

// LoadPodInfoCmd fetches pod information for a specific context and namespace
func LoadPodInfoCmd(client *k8s.Client, kubeContext, namespace string) tea.Cmd {
	return func() tea.Msg {
		// Fetch pods for the context
		pods, err := client.ListPodInfo(kubeContext, namespace)
		if err != nil {
			return msgs.PodTableMsg{
				Context: kubeContext,
				Rows:    nil,
				Err:     err,
			}
		}

		// Convert pods to table rows
		rows := make([]msgs.RowData, len(pods))
		for i, pod := range pods {
			rows[i] = msgs.RowData{
				msgs.PodKeyName:       pod.Name,
				msgs.PodKeyNamespace:  pod.Namespace,
				msgs.PodKeyStatus:     pod.Status,
				msgs.PodKeyRestarts:   strconv.FormatInt(int64(pod.Restarts), 10),
				msgs.PodKeyAge:        pod.Age,
				msgs.PodKeyContext:    pod.Context,                       // hidden, used by the detail tab
				msgs.PodKeyContainers: strings.Join(pod.Containers, ","), // hidden, used by the log pane
				msgs.PodKeyNode:       pod.Node,
				msgs.PodKeyNodeIP:     pod.NodeIP,
				msgs.PodKeyPodIP:      pod.PodIP,
				msgs.PodKeyReady:      pod.ReadyContainers,
			}
		}

		return msgs.PodTableMsg{
			Context: kubeContext,
			Rows:    rows,
			Err:     nil,
		}
	}
}

// LoadDeploymentInfoCmd fetches deployment information for a specific context and namespace
func LoadDeploymentInfoCmd(client *k8s.Client, kubeContext, namespace string) tea.Cmd {
	return func() tea.Msg {
		// Fetch deployments for the context
		deployments, err := client.GetDeploymentInfo(kubeContext, namespace)
		if err != nil {
			return msgs.DeploymentTableMsg{
				Context: kubeContext,
				Rows:    nil,
				Err:     err,
			}
		}

		// Convert deployments to table rows
		rows := make([]msgs.RowData, len(deployments))
		for i, deployment := range deployments {
			rows[i] = msgs.RowData{
				msgs.DeployKeyName:      deployment.Name,
				msgs.DeployKeyAge:       deployment.Age,
				msgs.DeployKeyReplicas:  strconv.Itoa(int(deployment.ReadyReplicas)) + "/" + strconv.Itoa(int(deployment.DesiredReplicas)),
				msgs.DeployKeyContext:   kubeContext,
				msgs.DeployKeyNamespace: deployment.Namespace, // hidden, used by the detail panel
				msgs.DeployKeyStrategy:  deployment.Strategy,
				msgs.DeployKeyAvailable: strconv.FormatInt(int64(deployment.AvailableReplicas), 10),
				msgs.DeployKeyUpdated:   strconv.FormatInt(int64(deployment.UpdatedReplicas), 10),
				msgs.DeployKeySelector:  deployment.Selector,
			}
		}

		return msgs.DeploymentTableMsg{
			Context: kubeContext,
			Rows:    rows,
			Err:     nil,
		}
	}
}

// LoadServiceInfoCmd fetches service information for a specific context and namespace
func LoadServiceInfoCmd(client *k8s.Client, kubeContext, namespace string) tea.Cmd {
	return func() tea.Msg {
		services, err := client.GetServiceInfo(kubeContext, namespace)
		if err != nil {
			return msgs.ServiceTableMsg{
				Context: kubeContext,
				Rows:    nil,
				Err:     err,
			}
		}

		rows := make([]msgs.RowData, len(services))
		for i, svc := range services {
			rows[i] = msgs.RowData{
				msgs.SvcKeyName:        svc.Name,
				msgs.SvcKeyNamespace:   svc.Namespace,
				msgs.SvcKeyType:        svc.Type,
				msgs.SvcKeyClusterIP:   svc.ClusterIP,
				msgs.SvcKeyPorts:       svc.Ports,
				msgs.SvcKeyAge:         svc.Age,
				msgs.SvcKeyContext:     kubeContext, // hidden, used by the detail tab
				msgs.SvcKeySelector:    svc.Selector,
				msgs.SvcKeyExternalIP:  svc.ExternalIP,
				msgs.SvcKeyEndpointIPs: endpointIPsPlaceholder, // replaced once LoadServiceEndpointsCmd resolves
			}
		}

		return msgs.ServiceTableMsg{
			Context: kubeContext,
			Rows:    rows,
			Err:     nil,
		}
	}
}

// endpointIPsPlaceholder is shown in the Endpoint IPs wide-mode column until
// LoadServiceEndpointsCmd's lazy fetch resolves for that context+namespace.
const endpointIPsPlaceholder = "…"

// LoadServiceEndpointsCmd fetches Endpoint IPs for every service in one
// context+namespace via a single EndpointSlices list call. It's triggered
// lazily (see mainPage.go's Ctrl+W handling for the svc tab), not folded
// into LoadServiceInfoCmd/refresh, so it only ever runs once per
// context+namespace until that namespace's selection changes.
func LoadServiceEndpointsCmd(client *k8s.Client, kubeContext, namespace string) tea.Cmd {
	return func() tea.Msg {
		endpoints, err := client.GetServiceEndpoints(kubeContext, namespace)
		if err != nil {
			return msgs.ServiceEndpointsMsg{Context: kubeContext, Namespace: namespace, Err: err}
		}
		return msgs.ServiceEndpointsMsg{Context: kubeContext, Namespace: namespace, Endpoints: endpoints}
	}
}

// LoadDeploymentDetailCmd fetches detailed information for a single deployment
func LoadDeploymentDetailCmd(client *k8s.Client, kubeContext, namespace, deploymentName string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetDeploymentDetail(kubeContext, namespace, deploymentName)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadPodDetailCmd fetches detailed information for a single pod
func LoadPodDetailCmd(client *k8s.Client, kubeContext, namespace, podName string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetPodDetail(kubeContext, namespace, podName)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// LoadServiceDetailCmd fetches detailed information for a single service
func LoadServiceDetailCmd(client *k8s.Client, kubeContext, namespace, serviceName string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.GetServiceDetail(kubeContext, namespace, serviceName)
		if err != nil {
			return msgs.ResourceDetailMsg{Context: kubeContext, Err: err}
		}
		return msgs.ResourceDetailMsg{Context: kubeContext, Detail: detail}
	}
}

// OpenPodLogStreamCmd opens a following log stream for a single pod
// container (one source in the merged Log pane), backfilled with the last
// logTailLines lines. sourceKey identifies which source this is, and
// generation is echoed back on the resulting message so the caller can
// tell whether this stream is still the one it's waiting for — that
// specific source may have been restarted or closed before this resolves,
// independent of any other open source.
func OpenPodLogStreamCmd(client *k8s.Client, kubeContext, namespace, podName, container, sourceKey string, generation int) tea.Cmd {
	return func() tea.Msg {
		opts := &v1.PodLogOptions{
			Follow:    true,
			TailLines: int64Ptr(logTailLines),
			Container: container,
		}
		stream, err := client.StreamLogs(kubeContext, namespace, podName, opts)
		if err != nil {
			return msgs.LogStreamClosedMsg{SourceKey: sourceKey, Generation: generation, Err: err}
		}
		return msgs.LogStreamOpenedMsg{SourceKey: sourceKey, Generation: generation, Stream: stream}
	}
}

// WaitForLogLineCmd reads the next line from scanner and returns it as a
// LogLineMsg, or a LogStreamClosedMsg once the stream ends (scanner.Err()
// is nil on a clean EOF). The caller re-issues this command after each
// LogLineMsg to keep that source's read loop going.
func WaitForLogLineCmd(sourceKey string, generation int, scanner *bufio.Scanner) tea.Cmd {
	return func() tea.Msg {
		if scanner.Scan() {
			return msgs.LogLineMsg{SourceKey: sourceKey, Generation: generation, Line: scanner.Text()}
		}
		return msgs.LogStreamClosedMsg{SourceKey: sourceKey, Generation: generation, Err: scanner.Err()}
	}
}

// NewLogScanner wraps an opened log stream in a bufio.Scanner sized to
// tolerate abnormally long individual log lines.
func NewLogScanner(stream io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLogLineBytes)
	return scanner
}
