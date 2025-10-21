package k8s

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeploymentInfo struct {
	Name   string
	Age    time.Duration
	Ready  string
	Status []string
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

		age := time.Since(deployment.CreationTimestamp.Time)

		deploymentInfoList = append(deploymentInfoList, DeploymentInfo{
			Name:  deployment.Name,
			Age:   age,
			Ready: deployment.Status.String(),
		})

	}
	return deploymentInfoList, nil
}
