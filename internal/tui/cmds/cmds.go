// Package cmds implement interface to k8s client
package cmds

import (
	"bufio"
	"io"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
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
		rows := make([]table.Row, len(pods))
		for i, pod := range pods {
			rows[i] = table.Row{
				pod.Name,
				pod.Namespace,
				pod.Status,
				strconv.FormatInt(int64(pod.Restarts), 10),
				pod.Age,
				pod.Context,                       // hidden column, used by the detail tab
				strings.Join(pod.Containers, ","), // hidden column, used by the log pane
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
		rows := make([]table.Row, len(deployments))
		for i, deployment := range deployments {
			rows[i] = table.Row{
				deployment.Name,
				deployment.Age,
				strconv.Itoa(int(deployment.ReadyReplicas)),
				kubeContext,
				deployment.Namespace, // hidden column, used by the detail panel
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

		rows := make([]table.Row, len(services))
		for i, svc := range services {
			rows[i] = table.Row{
				svc.Name,
				svc.Namespace,
				svc.Type,
				svc.ClusterIP,
				svc.Ports,
				svc.Age,
				kubeContext, // hidden column, used by the detail pane
			}
		}

		return msgs.ServiceTableMsg{
			Context: kubeContext,
			Rows:    rows,
			Err:     nil,
		}
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
// container, backfilled with the last logTailLines lines. sessionID is
// echoed back on the resulting message so the caller can tell whether this
// stream is still the one it's waiting for (the user may have switched pod
// or container, or closed the pane, before this resolves).
func OpenPodLogStreamCmd(client *k8s.Client, kubeContext, namespace, podName, container string, sessionID int) tea.Cmd {
	return func() tea.Msg {
		opts := &v1.PodLogOptions{
			Follow:    true,
			TailLines: int64Ptr(logTailLines),
			Container: container,
		}
		stream, err := client.StreamLogs(kubeContext, namespace, podName, opts)
		if err != nil {
			return msgs.LogStreamClosedMsg{SessionID: sessionID, Err: err}
		}
		return msgs.LogStreamOpenedMsg{SessionID: sessionID, Stream: stream}
	}
}

// WaitForLogLineCmd reads the next line from scanner and returns it as a
// LogLineMsg, or a LogStreamClosedMsg once the stream ends (scanner.Err()
// is nil on a clean EOF). The caller re-issues this command after each
// LogLineMsg to keep the read loop going.
func WaitForLogLineCmd(sessionID int, scanner *bufio.Scanner) tea.Cmd {
	return func() tea.Msg {
		if scanner.Scan() {
			return msgs.LogLineMsg{SessionID: sessionID, Line: scanner.Text()}
		}
		return msgs.LogStreamClosedMsg{SessionID: sessionID, Err: scanner.Err()}
	}
}

// NewLogScanner wraps an opened log stream in a bufio.Scanner sized to
// tolerate abnormally long individual log lines.
func NewLogScanner(stream io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLogLineBytes)
	return scanner
}
