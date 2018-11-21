package setup

import (
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/supergloo/cli/pkg/cmd/options"
	"github.com/solo-io/supergloo/cli/pkg/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func InitCache(opts *options.Options) error {

	// Get a kube client
	kube, err := common.GetKubernetesClient()
	if err != nil {
		return err
	}
	opts.Cache.KubeClient = kube

	// Get all namespaces
	list, err := kube.CoreV1().Namespaces().List(v1.ListOptions{IncludeUninitialized: false})
	if err != nil {
		return err
	}
	var namespaces = []string{}
	for _, ns := range list.Items {
		namespaces = append(namespaces, ns.ObjectMeta.Name)
	}
	opts.Cache.Namespaces = namespaces

	// Get key resources by ns
	meshClient, err := common.GetMeshClient()
	if err != nil {
		return err
	}
	opts.Cache.NsResources = make(map[string]options.NsResource)
	for _, ns := range namespaces {
		meshList, err := (*meshClient).List(ns, clients.ListOpts{})
		if err != nil {
			return err
		}
		var meshes = []string{}
		for _, m := range meshList {
			meshes = append(meshes, m.Metadata.Name)
		}
		opts.Cache.NsResources[ns] = options.NsResource{
			Meshes: meshes,
		}
	}

	return nil
}
