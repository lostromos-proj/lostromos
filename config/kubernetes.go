package config

import (
	"fmt"
	"path/filepath"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// NewKubernetesClient creates a Kubernetes client using the provided kubeconfig path.
// If kubeconfig is empty, it tries in-cluster config then falls back to the default
// system location (~/.kube/config).
func NewKubernetesClient(kubeconfig string) (kubernetes.Interface, error) {
	cfg, err := getKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return clientset, nil
}

// NewDynamicClient creates a dynamic Kubernetes client using the provided kubeconfig path.
// If kubeconfig is empty, it tries in-cluster config then falls back to the default
// system location (~/.kube/config).
func NewDynamicClient(kubeconfig string) (dynamic.Interface, error) {
	cfg, err := getKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return dynamicClient, nil
}

func getKubeConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig %q: %w", kubeconfig, err)
		}
		return cfg, nil
	}

	// No explicit kubeconfig: try in-cluster config, fall back to default system location.
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}

	defaultPath := filepath.Join(homedir.HomeDir(), ".kube", "config")
	cfg, err := clientcmd.BuildConfigFromFlags("", defaultPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client config: %w", err)
	}
	return cfg, nil
}
