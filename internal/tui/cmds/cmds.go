// Package cmds implement interface to k8s client
package cmds

import (
	"bufio"
	"io"

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

// endpointIPsPlaceholder is shown in the Endpoint IPs wide-mode column until
// LoadServiceEndpointsCmd's lazy fetch resolves for that context+namespace.
const endpointIPsPlaceholder = "..."

// LoadServiceEndpointsCmd fetches Endpoint IPs for every service in one
// context+namespace via a single EndpointSlices list call. It's triggered
// lazily (see mainPage.go's Ctrl+W handling for the svc tab), independent of
// the Services watch, so it only ever runs once per context+namespace until
// that namespace's selection changes.
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
