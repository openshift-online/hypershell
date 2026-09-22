package gateway

import (
	"context"
	"fmt"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const (
	gatewayHealthPortName = "health"
	// GatewayHealthPort is the gateway HTTP health port.
	GatewayHealthPort int32 = 8081
	// GatewayHealthServiceName is the controller-only health Service name.
	GatewayHealthServiceName   = "openshell-gateway-health"
	gatewayHealthAccessTimeout = 3 * time.Second
)

var gatewayManagedLabels = map[string]string{
	"app.kubernetes.io/name":       "openshell",
	"app.kubernetes.io/component":  "gateway",
	"app.kubernetes.io/managed-by": "hypershell-control-plane",
	"hypershell.redhat.io/managed": "true",
}

// ReconcileGatewayHealthAccess makes the internal gateway health endpoint
// available to the control plane. It reconciles the dedicated Service.
func ReconcileGatewayHealthAccess(ctx context.Context, clientset kubernetes.Interface, namespace string) error {
	if namespace == "" {
		return fmt.Errorf("gateway namespace is required")
	}

	return reconcileGatewayHealthService(ctx, clientset, namespace)
}

func reconcileGatewayHealthService(ctx context.Context, clientset kubernetes.Interface, namespace string) error {
	ctx, cancel := context.WithTimeout(ctx, gatewayHealthAccessTimeout)
	defer cancel()

	services := clientset.CoreV1().Services(namespace)
	desiredPorts := []corev1.ServicePort{{
		Name:       gatewayHealthPortName,
		Protocol:   corev1.ProtocolTCP,
		Port:       GatewayHealthPort,
		TargetPort: intstr.FromString(gatewayHealthPortName),
	}}
	desiredSelector := map[string]string{
		"app.kubernetes.io/instance": GatewayDeploymentName,
		"app.kubernetes.io/name":     "openshell",
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		service, err := services.Get(ctx, GatewayHealthServiceName, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			_, err = services.Create(ctx, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      GatewayHealthServiceName,
					Namespace: namespace,
					Labels:    copyStringMap(gatewayManagedLabels),
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeClusterIP,
					Selector: desiredSelector,
					Ports:    desiredPorts,
				},
			}, metav1.CreateOptions{})
			return err
		}
		if err != nil {
			return err
		}

		updated := service.DeepCopy()
		changed := false
		if updated.Spec.Type != corev1.ServiceTypeClusterIP {
			updated.Spec.Type = corev1.ServiceTypeClusterIP
			changed = true
		}
		if len(updated.Spec.ExternalIPs) != 0 {
			updated.Spec.ExternalIPs = nil
			changed = true
		}
		if updated.Spec.LoadBalancerIP != "" {
			updated.Spec.LoadBalancerIP = ""
			changed = true
		}
		if len(updated.Spec.LoadBalancerSourceRanges) != 0 {
			updated.Spec.LoadBalancerSourceRanges = nil
			changed = true
		}
		if updated.Spec.LoadBalancerClass != nil {
			updated.Spec.LoadBalancerClass = nil
			changed = true
		}
		if updated.Spec.ExternalName != "" {
			updated.Spec.ExternalName = ""
			changed = true
		}
		if updated.Spec.ExternalTrafficPolicy != "" {
			updated.Spec.ExternalTrafficPolicy = ""
			changed = true
		}
		if updated.Spec.HealthCheckNodePort != 0 {
			updated.Spec.HealthCheckNodePort = 0
			changed = true
		}
		if updated.Spec.AllocateLoadBalancerNodePorts != nil {
			updated.Spec.AllocateLoadBalancerNodePorts = nil
			changed = true
		}
		if !reflect.DeepEqual(updated.Spec.Selector, desiredSelector) {
			updated.Spec.Selector = desiredSelector
			changed = true
		}
		if !reflect.DeepEqual(updated.Spec.Ports, desiredPorts) {
			updated.Spec.Ports = desiredPorts
			changed = true
		}
		if updated.Labels == nil {
			updated.Labels = make(map[string]string, len(gatewayManagedLabels))
		}
		for key, value := range gatewayManagedLabels {
			if updated.Labels[key] != value {
				updated.Labels[key] = value
				changed = true
			}
		}
		if !changed {
			return nil
		}

		_, err = services.Update(ctx, updated, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return fmt.Errorf("reconcile gateway health Service in %s: %w", namespace, err)
	}
	return nil
}

func copyStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
