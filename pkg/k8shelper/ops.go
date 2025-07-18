package k8shelper

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Ops help you do various operations in your kubernetes cluster
type Ops struct {
	kClient *kubernetes.Clientset
	logger  *zap.Logger
}

// NewOps create a new instance for the k8s Operations
func NewOps(logger *zap.Logger, client *kubernetes.Clientset) *Ops {
	return &Ops{
		logger:  logger.Named("k8sOps"),
		kClient: client,
	}
}

// CheckIfServiceEndpointActive returns true if endpoint for a service is active
func (k *Ops) CheckIfServiceEndpointActive(ns, svc string) (bool, error) {
	endpoint, err := k.kClient.CoreV1().Endpoints(ns).Get(context.TODO(), svc, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("CheckIfServiceEndpointActive - GET: %w", err)
	}

	if len(endpoint.Subsets) > 0 {
		if len(endpoint.Subsets[0].Addresses) != 0 {
			k.logger.Debug("Service endpoint is active", zap.String("service", svc), zap.String("namespace", ns))
			return true, nil
		}
	}
	return false, nil
}
