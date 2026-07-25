package runtime

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (k *KubernetesRuntime) Bootstrap(ctx context.Context) error {
	if _, err := k.client.CoreV1().Namespaces().Get(ctx, k.config.Namespace, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err := k.client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: k.config.Namespace, Labels: map[string]string{"app.kubernetes.io/managed-by": "norbot"}}}, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create namespace: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get namespace: %w", err)
	}
	serviceAccounts := k.client.CoreV1().ServiceAccounts(k.config.Namespace)
	if _, err := serviceAccounts.Get(ctx, k.config.ServiceAccount, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err := serviceAccounts.Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: k.config.ServiceAccount, Labels: map[string]string{"app.kubernetes.io/managed-by": "norbot"}}}, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create service account: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get service account: %w", err)
	}
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "norbot-runtime", Labels: map[string]string{"app.kubernetes.io/managed-by": "norbot"}}, Rules: []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods", "pods/log", "pods/exec", "persistentvolumeclaims", "configmaps", "services"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"}},
		{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
		{APIGroups: []string{"batch"}, Resources: []string{"jobs"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"}},
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
		{APIGroups: []string{"networking.k8s.io"}, Resources: []string{"ingresses", "networkpolicies"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
	}}
	roles := k.client.RbacV1().Roles(k.config.Namespace)
	if _, err := roles.Create(ctx, role, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create role: %w", err)
	}
	binding := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "norbot-runtime", Labels: map[string]string{"app.kubernetes.io/managed-by": "norbot"}}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "norbot-runtime"}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: k.config.ServiceAccount, Namespace: k.config.Namespace}}}
	bindings := k.client.RbacV1().RoleBindings(k.config.Namespace)
	if _, err := bindings.Create(ctx, binding, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create role binding: %w", err)
	}
	return nil
}

func RegistrySecretTemplate(name string) string {
	return "apiVersion: v1\nkind: Secret\nmetadata:\n  name: " + name + "\ntype: kubernetes.io/dockerconfigjson\nstringData:\n  .dockerconfigjson: |\n    {\\\"auths\\\": {}}\n"
}
