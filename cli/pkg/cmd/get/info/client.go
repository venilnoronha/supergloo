package info

import (
	"fmt"
	"strings"

	"github.com/solo-io/solo-kit/pkg/errors"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/supergloo/cli/pkg/common"
	sgConstants "github.com/solo-io/supergloo/pkg/constants"

	"github.com/solo-io/solo-kit/pkg/utils/kubeutils"
	superglooV1 "github.com/solo-io/supergloo/pkg/api/v1"
	k8sApiExt "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	k8s "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

type SuperglooInfoClient interface {
	ListResourceTypes() ([]string, error)
	ListResources(resourceType, resourceName string) (*ResourceInfo, error)
}

type KubernetesInfoClient struct {
	kubeCrdClient      *k8sApiExt.CustomResourceDefinitionInterface
	meshClient         *superglooV1.MeshClient
	routingRulesClient *superglooV1.RoutingRuleClient
}

func NewClient() (SuperglooInfoClient, error) {
	config, err := kubeutils.GetConfig("", "")
	if err != nil {
		return nil, fmt.Errorf(common.KubeConfigError, err)
	}

	crdClient, err := getCrdClient(config)
	if err != nil {
		return nil, err
	}

	meshClient, err := common.GetMeshClient()
	if err != nil {
		return nil, err
	}

	rrClient, err := common.GetRoutingRuleClient()
	if err != nil {
		return nil, err
	}

	client := &KubernetesInfoClient{
		kubeCrdClient:      crdClient,
		meshClient:         meshClient,
		routingRulesClient: rrClient,
	}

	return client, nil
}

// Get a list of all supergloo resources
func (client *KubernetesInfoClient) ListResourceTypes() ([]string, error) {
	crdList, err := (*client.kubeCrdClient).List(k8s.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("Error retrieving supergloo resource types. Cause: %v \n", err)
	}

	superglooCRDs := make([]string, 0)
	for _, crd := range crdList.Items {
		if strings.Contains(crd.Name, common.SuperglooGroupName) {
			parts := strings.Split(crd.Name, ".")
			if len(parts) > 0 {
				superglooCRDs = append(superglooCRDs, parts[0])
			}
		}
	}
	return superglooCRDs, nil
}

func (client *KubernetesInfoClient) ListResources(resourceType, resourceName string) (*ResourceInfo, error) {
	// TODO(marco): make code more generic. Ideally we don't want to enumerate the different options, but I could not
	// find an interface that all of the generated clients implement
	switch resourceType {
	case "meshes":
		if resourceName == "" {
			meshList, err := (*client.meshClient).List(sgConstants.SuperglooNamespace, clients.ListOpts{})
			if err != nil {
				return nil, err
			}
			return FromMeshList(&meshList), nil
		} else {
			mesh, err := (*client.meshClient).Read(sgConstants.SuperglooNamespace, resourceName, clients.ReadOpts{})
			if err != nil {
				return nil, err
			}
			return FromMesh(mesh), nil
		}
	case "routingrules":
		if resourceName == "" {
			// TODO: replace with supergloo namespace
			rrList, err := (*client.routingRulesClient).List("default", clients.ListOpts{})
			if err != nil {
				return nil, err
			}
			return FromRoutingRuleList(&rrList), nil
		} else {
			rr, err := (*client.routingRulesClient).Read("default", resourceName, clients.ReadOpts{})
			if err != nil {
				return nil, err
			}
			return FromRoutingRule(rr), nil
		}
	default:
		// Should not happen since we validate the resource
		return nil, errors.Errorf(common.UnknownResourceTypeMsg, resourceType)
	}
}

// Return a client to query kubernetes CRDs
func getCrdClient(config *rest.Config) (*k8sApiExt.CustomResourceDefinitionInterface, error) {
	apiExtClient, err := k8sApiExt.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("Error building Kubernetes client: %v \n", err)
	}
	crdClient := apiExtClient.CustomResourceDefinitions()
	return &crdClient, nil
}
