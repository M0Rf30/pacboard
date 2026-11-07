// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package kube watches Pipelines-as-Code and Tekton objects through dynamic informers and
// serves them from in-memory indexes; it only ever reads from the cluster.
package kube

import (
	"fmt"
	"os"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	clientQPS   = 50
	clientBurst = 100
)

// RestConfig loads the cluster configuration: the kubeconfig (explicit path, KUBECONFIG or
// ~/.kube/config, with an optional context) when one is asked for or no in-cluster
// environment exists, else the in-cluster service account.
func RestConfig(kubeconfig, kubeContext string) (*rest.Config, error) {
	explicit := kubeconfig != "" || kubeContext != "" || os.Getenv("KUBECONFIG") != ""

	if !explicit && os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("in-cluster config: %w", err)
		}

		return limits(cfg), nil
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = kubeconfig

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules, &clientcmd.ConfigOverrides{CurrentContext: kubeContext}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kubeconfig: %w", err)
	}

	return limits(cfg), nil
}

func limits(cfg *rest.Config) *rest.Config {
	cfg.QPS = clientQPS
	cfg.Burst = clientBurst
	cfg.UserAgent = "pacboard"

	return cfg
}
