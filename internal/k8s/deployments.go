package k8s

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeploymentInfo struct {
	Name          string
	Age           string
	ReadyReplicas int32
	Status        []string
}

func (c *Client) GetDeploymentInfo(kubeContextName, namespace string) ([]DeploymentInfo, error) {
	if kubeContextName != c.GetCurrentContext() {
		c.SwitchContext(kubeContextName)
	}
	d, err := c.clientset.AppsV1().Deployments(namespace).List(context.Background(), v1.ListOptions{})
	deploymentInfoList := []DeploymentInfo{}
	if err != nil {
		return []DeploymentInfo{}, fmt.Errorf("error list Deployments %v", err)
	}
	for _, deployment := range d.Items {

		age := formatDuration(time.Since(deployment.CreationTimestamp.Time))
		ReadyReplicas := deployment.Status.ReadyReplicas

		deploymentInfoList = append(deploymentInfoList, DeploymentInfo{
			Name:          deployment.Name,
			Age:           age,
			ReadyReplicas: ReadyReplicas,
		})

	}
	return deploymentInfoList, nil
}
