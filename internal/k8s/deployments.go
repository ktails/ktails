package k8s

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type DeploymentInfo struct {
	Name              string
	Namespace         string
	Age               string
	ReadyReplicas     int32
	DesiredReplicas   int32
	AvailableReplicas int32
	UpdatedReplicas   int32
	Strategy          string
	Selector          string
	Status            []string
}

// DeploymentToDeploymentInfo converts a deployment object to DeploymentInfo.
func DeploymentToDeploymentInfo(deployment *appsv1.Deployment) DeploymentInfo {
	age := formatDuration(time.Since(deployment.CreationTimestamp.Time))

	// Spec.Replicas is nil when unset, which the Kubernetes API defaults to 1.
	desiredReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		desiredReplicas = *deployment.Spec.Replicas
	}

	return DeploymentInfo{
		Name:              deployment.Name,
		Namespace:         deployment.Namespace,
		Age:               age,
		ReadyReplicas:     deployment.Status.ReadyReplicas,
		DesiredReplicas:   desiredReplicas,
		AvailableReplicas: deployment.Status.AvailableReplicas,
		UpdatedReplicas:   deployment.Status.UpdatedReplicas,
		Strategy:          string(deployment.Spec.Strategy.Type),
		Selector:          v1.FormatLabelSelector(deployment.Spec.Selector),
		Status:            []string{}, // You can add status conditions here if needed
	}
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
