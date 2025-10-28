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

// GetDeploymentInfo retrieves deployment information for a specific context and namespace
func (c *Client) GetDeploymentInfo(kubeContextName, namespace string) ([]DeploymentInfo, error) {
	// Get the appropriate client for this context
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	// List deployments
	deploymentList, err := clientset.AppsV1().Deployments(namespace).List(
		context.Background(),
		v1.ListOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments in namespace %s (context %s): %w", 
			namespace, kubeContextName, err)
	}

	// Convert to DeploymentInfo
	deploymentInfoList := make([]DeploymentInfo, 0, len(deploymentList.Items))
	for _, deployment := range deploymentList.Items {
		age := formatDuration(time.Since(deployment.CreationTimestamp.Time))
		
		deploymentInfoList = append(deploymentInfoList, DeploymentInfo{
			Name:          deployment.Name,
			Age:           age,
			ReadyReplicas: deployment.Status.ReadyReplicas,
			Status:        []string{}, // You can add status conditions here if needed
		})
	}

	return deploymentInfoList, nil
}