package runtime

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
	if err := k.ensureNamespaceGuardrails(ctx); err != nil {
		return err
	}
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "norbot-runtime", Labels: map[string]string{"app.kubernetes.io/managed-by": "norbot"}}, Rules: []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"pods", "pods/log", "pods/exec", "persistentvolumeclaims", "configmaps", "services"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"}},
		{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}},
		{APIGroups: []string{"batch"}, Resources: []string{"jobs"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"}},
		{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
		{APIGroups: []string{"networking.k8s.io"}, Resources: []string{"ingresses", "networkpolicies"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
	}}
	roles := k.client.RbacV1().Roles(k.config.Namespace)
	if _, err := roles.Create(ctx, role, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create role: %w", err)
		}
		existing, getErr := roles.Get(ctx, role.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("get role: %w", getErr)
		}
		existing.Labels = role.Labels
		existing.Rules = role.Rules
		if _, err := roles.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update role: %w", err)
		}
	}
	binding := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "norbot-runtime", Labels: map[string]string{"app.kubernetes.io/managed-by": "norbot"}}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "norbot-runtime"}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: k.config.ServiceAccount, Namespace: k.config.Namespace}}}
	bindings := k.client.RbacV1().RoleBindings(k.config.Namespace)
	if _, err := bindings.Create(ctx, binding, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create role binding: %w", err)
		}
		existing, getErr := bindings.Get(ctx, binding.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("get role binding: %w", getErr)
		}
		existing.Labels = binding.Labels
		existing.RoleRef = binding.RoleRef
		existing.Subjects = binding.Subjects
		if _, err := bindings.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update role binding: %w", err)
		}
	}
	if _, err := k.client.CoreV1().Secrets(k.config.Namespace).Get(ctx, k.config.RegistryPullSecret, metav1.GetOptions{}); err != nil {
		return fmt.Errorf("get registry pull secret %q: %w", k.config.RegistryPullSecret, err)
	}
	if k.config.EgressProxyImage != "" {
		secret, err := k.client.CoreV1().Secrets(k.config.Namespace).Get(ctx, k.config.EgressProxySecret, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get egress proxy secret %q: %w", k.config.EgressProxySecret, err)
		}
		if len(secret.Data[k.config.EgressProxySecretKey]) == 0 {
			return fmt.Errorf("egress proxy secret %q has no %q key", k.config.EgressProxySecret, k.config.EgressProxySecretKey)
		}
		if err := k.ensureEgressProxy(ctx); err != nil {
			return err
		}
		if err := k.waitDeployment(ctx, k.EgressProxyService()); err != nil {
			return fmt.Errorf("wait for egress proxy: %w", err)
		}
	}
	return nil
}

func (k *KubernetesRuntime) ensureNamespaceGuardrails(ctx context.Context) error {
	labels := map[string]string{"app.kubernetes.io/managed-by": "norbot"}
	quota := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "norbot-runtime", Labels: labels}, Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
		corev1.ResourceRequestsCPU:              resource.MustParse(fmt.Sprintf("%dm", k.config.QuotaCPUMilli)),
		corev1.ResourceLimitsCPU:                resource.MustParse(fmt.Sprintf("%dm", k.config.QuotaCPUMilli)),
		corev1.ResourceRequestsMemory:           resource.MustParse(fmt.Sprintf("%dMi", k.config.QuotaMemoryMiB)),
		corev1.ResourceLimitsMemory:             resource.MustParse(fmt.Sprintf("%dMi", k.config.QuotaMemoryMiB)),
		corev1.ResourceRequestsStorage:          resource.MustParse(fmt.Sprintf("%dGi", k.config.QuotaStorageGiB)),
		corev1.ResourcePods:                     resource.MustParse(strconv.FormatInt(k.config.QuotaPods, 10)),
		corev1.ResourcePersistentVolumeClaims:   resource.MustParse(strconv.FormatInt(k.config.QuotaPVCs, 10)),
		corev1.ResourceName("count/jobs.batch"): resource.MustParse(strconv.FormatInt(k.config.QuotaJobs, 10)),
	}}}
	quotas := k.client.CoreV1().ResourceQuotas(k.config.Namespace)
	if _, err := quotas.Create(ctx, quota, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create resource quota: %w", err)
		}
		existing, getErr := quotas.Get(ctx, quota.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("get resource quota: %w", getErr)
		}
		existing.Labels, existing.Spec = quota.Labels, quota.Spec
		if _, err := quotas.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update resource quota: %w", err)
		}
	}
	limitRange := &corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "norbot-runtime", Labels: labels}, Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{
		{Type: corev1.LimitTypeContainer, DefaultRequest: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(fmt.Sprintf("%dm", k.config.CPUMilli)), corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", k.config.MemoryMiB))}, Default: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(fmt.Sprintf("%dm", k.config.CPUMilli)), corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", k.config.MemoryMiB))}, Max: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(fmt.Sprintf("%dm", k.config.QuotaCPUMilli)), corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", k.config.QuotaMemoryMiB))}},
		{Type: corev1.LimitTypePersistentVolumeClaim, Min: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}, Max: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(fmt.Sprintf("%dGi", k.config.QuotaStorageGiB))}},
	}}}
	ranges := k.client.CoreV1().LimitRanges(k.config.Namespace)
	if _, err := ranges.Create(ctx, limitRange, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create limit range: %w", err)
		}
		existing, getErr := ranges.Get(ctx, limitRange.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("get limit range: %w", getErr)
		}
		existing.Labels, existing.Spec = limitRange.Labels, limitRange.Spec
		if _, err := ranges.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update limit range: %w", err)
		}
	}
	return nil
}

func RegistrySecretTemplate(name string) string {
	return "apiVersion: v1\nkind: Secret\nmetadata:\n  name: " + name + "\ntype: kubernetes.io/dockerconfigjson\nstringData:\n  .dockerconfigjson: |\n    {\\\"auths\\\": {}}\n"
}
