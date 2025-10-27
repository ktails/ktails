// Package cmds implement interface to k8s client
package cmds

import (
	"strconv"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ktails/ktails/internal/k8s"
	"github.com/ktails/ktails/internal/tui/msgs"
)

func LoadPodInfoCmd(client *k8s.Client, kubeContext, namespace string) tea.Cmd {
	return func() tea.Msg {
		// Fetch pods for the context
		pods, err := client.ListPodInfo(kubeContext, namespace)
		if err != nil {
			return nil
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
			}
		}
		pt := msgs.PodTableMsg{Context: kubeContext, Rows: rows}
		return pt
	}
}

func LoadDeploymentInfoCmd(client *k8s.Client, kubeContext, namespace string) tea.Cmd {
	return func() tea.Msg {
		// Fetch deployments for the context
		deployments, err := client.GetDeploymentInfo(kubeContext, namespace)
		if err != nil {
			return nil
		}

		// Convert pods to table rows
		rows := make([]table.Row, len(deployments))
		for i, deployment := range deployments {
			rows[i] = table.Row{
				deployment.Name,
				deployment.Age,
				strconv.Itoa(int(deployment.ReadyReplicas)),
				kubeContext,
			}
		}
		pt := msgs.DeploymentTableMsg{Context: kubeContext, Rows: rows}
		return pt
	}
}
