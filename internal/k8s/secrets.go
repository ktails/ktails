package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// redactedValue replaces every Secret value shown to the user — table rows
// never see raw values at all (only key names), and the Detail Pane's YAML
// is redacted before rendering. A terminal's scrollback is not a safe place
// for cluster secrets, even base64-encoded ones.
const redactedValue = "<redacted>"

// SecretInfo contains Secret metadata. Values are deliberately never
// carried here — see redactedValue.
type SecretInfo struct {
	Name      string
	Namespace string
	Type      string
	Age       string
	Keys      []string
}

// SecretToSecretInfo converts a Secret object to SecretInfo, carrying only
// key names.
func SecretToSecretInfo(secret *corev1.Secret) SecretInfo {
	keys := make([]string, 0, len(secret.Data)+len(secret.StringData))
	for k := range secret.Data {
		keys = append(keys, k)
	}
	for k := range secret.StringData {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return SecretInfo{
		Name:      secret.Name,
		Namespace: secret.Namespace,
		Type:      string(secret.Type),
		Age:       formatDuration(time.Since(secret.CreationTimestamp.Time)),
		Keys:      keys,
	}
}

// secretsLW binds a SecretInterface's List/Watch to namespace, for
// watchResource/listResource — see generic.go.
func secretsLW(namespace string) func(kubernetes.Interface) listerWatcher[*corev1.SecretList] {
	return func(cs kubernetes.Interface) listerWatcher[*corev1.SecretList] {
		return cs.CoreV1().Secrets(namespace)
	}
}

// WatchSecrets opens a watch on Secrets in the given namespace. See WatchPods
// for the implicit list-then-watch behavior.
func (c *Client) WatchSecrets(ctx context.Context, kubeContext, namespace string) (watch.Interface, error) {
	return watchResource(ctx, c, kubeContext, "secrets in namespace "+namespace, secretsLW(namespace))
}

// ListSecrets fetches every Secret in the given namespace in one call. See
// ListPods for why this exists alongside the watch.
func (c *Client) ListSecrets(ctx context.Context, kubeContext, namespace string) ([]*corev1.Secret, error) {
	return listResource(ctx, c, kubeContext, "secrets in namespace "+namespace, secretsLW(namespace),
		func(l *corev1.SecretList) []*corev1.Secret { return pointers(l.Items) })
}

// GetSecretDetail fetches a single Secret's key names, redacted YAML, and
// recent events. Data/StringData values are replaced with redactedValue
// before rendering — never the real (even if base64-encoded) contents.
func (c *Client) GetSecretDetail(kubeContextName, namespace, name string) (ResourceDetail, error) {
	d := ResourceDetail{Kind: "Secret"}
	clientset, err := c.GetClientForContext(kubeContextName)
	if err != nil {
		return d, fmt.Errorf("failed to get client for context %s: %w", kubeContextName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return d, fmt.Errorf("failed to get secret %s in namespace %s (context %s): %w", name, namespace, kubeContextName, err)
	}

	info := SecretToSecretInfo(secret)
	d.Name = info.Name
	d.Namespace = info.Namespace
	d.Age = info.Age
	d.Summary = fmt.Sprintf("Type: %s  Keys: %s", info.Type, strings.Join(info.Keys, ", "))

	for k := range secret.Data {
		secret.Data[k] = []byte(redactedValue)
	}
	for k := range secret.StringData {
		secret.StringData[k] = redactedValue
	}
	d.YAML = renderDetailYAML(secret, "v1", "Secret")

	if events, err := c.getEvents(kubeContextName, namespace, "Secret", name); err == nil {
		d.Events = events
	}

	return d, nil
}
