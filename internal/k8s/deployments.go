package k8s

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type DeploymentInfo struct {
	Name            string
	Namespace       string
	Age             string
	ReadyReplicas   int32
	DesiredReplicas int32
	Status          []string
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

		// Spec.Replicas is nil when unset, which the Kubernetes API defaults to 1.
		desiredReplicas := int32(1)
		if deployment.Spec.Replicas != nil {
			desiredReplicas = *deployment.Spec.Replicas
		}

		deploymentInfoList = append(deploymentInfoList, DeploymentInfo{
			Name:            deployment.Name,
			Namespace:       deployment.Namespace,
			Age:             age,
			ReadyReplicas:   deployment.Status.ReadyReplicas,
			DesiredReplicas: desiredReplicas,
			Status:          []string{}, // You can add status conditions here if needed
		})
	}

	return deploymentInfoList, nil
}

// GetDeploymentDetail fetches a single deployment's status, rendered YAML, and recent events.
func (c *Client) GetDeploymentDetail(kubeContextName, namespace, deploymentName string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "Deployment"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.Background(), deploymentName, v1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get deployment %s in namespace %s (context %s): %w",
			deploymentName, namespace, kubeContextName, err)
	}

	d.Name = deployment.Name
	d.Namespace = deployment.Namespace
	d.Age = formatDuration(time.Since(deployment.CreationTimestamp.Time))
	d.Summary = fmt.Sprintf("Ready Replicas: %d", deployment.Status.ReadyReplicas)
	for _, condition := range deployment.Status.Conditions {
		d.Status = append(d.Status, formatCondition(string(condition.Type), string(condition.Status), condition.Reason, condition.Message))
	}

	// Render clean YAML the way `kubectl get -o yaml` would, minus noisy managed fields.
	deployment.ManagedFields = nil
	deployment.TypeMeta = v1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}
	if yamlBytes, yamlErr := yaml.Marshal(deployment); yamlErr == nil {
		d.YAML = string(yamlBytes)
	} else {
		d.YAML = fmt.Sprintf("failed to render YAML: %v", yamlErr)
	}

	if events, err := c.getEvents(kubeContextName, namespace, "Deployment", deploymentName); err == nil {
		d.Events = events
	}

	return d, nil
}
