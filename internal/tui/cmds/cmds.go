package cmds

import (
	"strconv"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ivyascorp-net/ktails/internal/k8s"
	"github.com/ivyascorp-net/ktails/internal/tui/msgs"
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
