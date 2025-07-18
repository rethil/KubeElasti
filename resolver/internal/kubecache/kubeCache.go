package kubecache

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	core_v1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type KubeCache struct {
	logger    *zap.Logger
	clientset *kubernetes.Clientset

	ingressCache sync.Map
	serviceCache sync.Map
}

func NewKubeCache(logger *zap.Logger, clientset *kubernetes.Clientset) *KubeCache {
	return &KubeCache{
		logger:    logger,
		clientset: clientset,

		ingressCache: sync.Map{},
		serviceCache: sync.Map{},
	}
}

func (kc *KubeCache) GetServiceForRequest(req *http.Request) (cache.ObjectName, bool) {
	services := []cache.ObjectName{}

	kc.ingressCache.Range(func(key any, value any) bool {
		ingressID := key.(*cache.ObjectName)
		ingressSpec := value.(*networking_v1.IngressSpec)

		for _, rule := range ingressSpec.Rules {
			if rule.Host != req.Host {
				continue
			}

			for _, path := range rule.HTTP.Paths {
				serviceId := cache.ObjectName{
					Name:      path.Backend.Service.Name,
					Namespace: ingressID.Namespace,
				}

				switch *path.PathType {
				case networking_v1.PathTypePrefix:
					if strings.HasPrefix(req.URL.Path, path.Path) {

						services = append(services, serviceId)
					}
				case networking_v1.PathTypeExact:
					if req.URL.Path == path.Path {
						services = append(services, serviceId)
					}
				}
			}
		}

		return false
	})

	existingServices := []cache.ObjectName{}

	for _, serviceID := range services {
		_, ok := kc.serviceCache.Load(serviceID)
		if ok {
			existingServices = append(existingServices, serviceID)
		}
	}

	if len(existingServices) == 0 {
		return cache.ObjectName{}, false
	}

	return existingServices[0], true
}

func (kc *KubeCache) Start(ctx context.Context) error {
	err := kc.watchIngresses(ctx)
	if err != nil {
		return err
	}

	err = kc.watchServices(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (kc *KubeCache) watchIngresses(ctx context.Context) error {
	ingressList, err := kc.clientset.NetworkingV1().Ingresses("").List(ctx, meta_v1.ListOptions{})
	if err != nil {
		return err
	}

	for _, ingress := range ingressList.Items {
		key := cache.MetaObjectToName(&ingress)

		kc.ingressCache.Store(key, ingress.Spec)
	}

	ingressWatch, err := kc.clientset.NetworkingV1().Ingresses("").Watch(ctx, meta_v1.ListOptions{})
	if err != nil {
		return err
	}

	go func() {
		for event := range ingressWatch.ResultChan() {
			ingress := event.Object.(*networking_v1.Ingress)
			key := cache.MetaObjectToName(ingress)

			switch event.Type {
			case watch.Added:
			case watch.Modified:
				kc.ingressCache.Store(key, ingress.Spec)

			case watch.Deleted:
				kc.ingressCache.Delete(key)
			}
		}
	}()

	return nil
}

func (kc *KubeCache) watchServices(ctx context.Context) error {
	serviceList, err := kc.clientset.CoreV1().Services("").List(ctx, meta_v1.ListOptions{})
	if err != nil {
		return err
	}

	for _, service := range serviceList.Items {
		key := cache.MetaObjectToName(&service)

		kc.serviceCache.Store(key, struct{}{})
	}

	serviceWatch, err := kc.clientset.CoreV1().Services("").Watch(ctx, meta_v1.ListOptions{})
	if err != nil {
		return err
	}

	go func() {
		for event := range serviceWatch.ResultChan() {
			service := event.Object.(*core_v1.Service)
			key := cache.MetaObjectToName(service)

			switch event.Type {
			case watch.Added:
			case watch.Modified:
				kc.ingressCache.Store(key, struct{}{})

			case watch.Deleted:
				kc.ingressCache.Delete(key)
			}
		}
	}()

	return nil
}
