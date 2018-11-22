package shared

import (
	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"k8s.io/api/admissionregistration/v1beta1"
	appsv1beta2 "k8s.io/api/apps/v1beta2"
	autoscaling "k8s.io/api/autoscaling/v1"
	batch "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	appsv1beta1 "k8s.io/api/extensions/v1beta1"
	rbac "k8s.io/api/rbac/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiexts "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"strings"
)

type UntypedKubeObject map[string]interface{}
type KubeObjectList []runtime.Object

func ParseKubeManifest(manifest string) (KubeObjectList, error) {
	snippets := strings.Split(manifest, "---")
	var objs KubeObjectList
	for _, objectYaml := range snippets {
		parsedObjs, err := parseobjectYaml(objectYaml)
		if err != nil {
			return nil, err
		}
		if parsedObjs == nil {
			continue
		}
		objs = append(objs, parsedObjs...)
	}
	return objs, nil
}

func parseobjectYaml(objectYaml string) (KubeObjectList, error) {
	obj, err := convertYamlToResource(objectYaml)
	if err != nil {
		return nil, errors.Wrapf(err, "unsupported object type: %v", objectYaml)
	}

	return obj, nil
}

func convertYamlToResource(objectYaml string) (KubeObjectList, error) {
	var untyped UntypedKubeObject
	if err := yaml.Unmarshal([]byte(objectYaml), &untyped); err != nil {
		return nil, errors.Wrapf(err, "unmarshalling %v", objectYaml)
	}
	// yaml was empty
	if untyped == nil {
		return nil, nil
	}

	// need to be done manually as the go structs are embedded
	var typeMeta metav1.TypeMeta
	if err := yaml.Unmarshal([]byte(objectYaml), &typeMeta); err != nil {
		return nil, errors.Wrapf(err, "parsing raw yaml as %+v", typeMeta)
	}

	kind := typeMeta.Kind

	var obj runtime.Object
	switch kind {
	case "List":
		return convertUntypedList(untyped)
	case "Namespace":
		obj = &core.Namespace{TypeMeta: typeMeta}
	case "ServiceAccount":
		obj = &core.ServiceAccount{TypeMeta: typeMeta}
	case "ClusterRole":
		obj = &rbac.ClusterRole{TypeMeta: typeMeta}
	case "ClusterRoleBinding":
		obj = &rbac.ClusterRoleBinding{TypeMeta: typeMeta}
	case "Job":
		obj = &batch.Job{TypeMeta: typeMeta}
	case "ConfigMap":
		obj = &core.ConfigMap{TypeMeta: typeMeta}
	case "Service":
		obj = &core.Service{TypeMeta: typeMeta}
	case "Deployment":
		obj = &appsv1beta2.Deployment{TypeMeta: typeMeta}
	case "DaemonSet":
		if typeMeta.APIVersion == "extensions/v1beta1" {
			obj = &appsv1beta1.DaemonSet{TypeMeta: typeMeta}
		} else {
			obj = &appsv1beta2.DaemonSet{TypeMeta: typeMeta}
		}
	case "CustomResourceDefinition":
		obj = &apiextensions.CustomResourceDefinition{TypeMeta: typeMeta}
	case "MutatingWebhookConfiguration":
		obj = &v1beta1.MutatingWebhookConfiguration{TypeMeta: typeMeta}
	case "HorizontalPodAutoscaler":
		obj = &autoscaling.HorizontalPodAutoscaler{TypeMeta: typeMeta}
	default:
		return nil, errors.Errorf("unsupported kind %v", kind)
	}
	if err := yaml.Unmarshal([]byte(objectYaml), obj); err != nil {
		return nil, errors.Wrapf(err, "parsing raw yaml as %+v", obj)
	}
	return KubeObjectList{obj}, nil
}
func convertUntypedList(untyped UntypedKubeObject) (KubeObjectList, error) {
	itemsValue, ok := untyped["items"]
	if !ok {
		return nil, errors.Errorf("list object missing items")
	}
	items, ok := itemsValue.([]interface{})
	if !ok {
		return nil, errors.Errorf("items must be an array")
	}

	var returnList KubeObjectList
	for _, item := range items {
		itemYaml, err := yaml.Marshal(item)
		if err != nil {
			return nil, errors.Wrapf(err, "marshalling item yaml")
		}
		s := string(itemYaml)
		obj, err := convertYamlToResource(s)
		if err != nil {
			return nil, errors.Wrapf(err, "converting resource in list")
		}
		returnList = append(returnList, obj...)
	}
	return returnList, nil
}

type kubeInterface struct {
	kube kubernetes.Interface
	exts apiexts.Interface
}

func (k *kubeInterface) create(obj runtime.Object) error {
	kube := k.kube
	exts := k.exts
	switch obj := obj.(type) {
	case *core.Namespace:
		_, err := kube.CoreV1().Namespaces().Create(obj)
		return err
	case *core.ConfigMap:
		_, err := kube.CoreV1().ConfigMaps(obj.Namespace).Create(obj)
		return err
	case *core.ServiceAccount:
		_, err := kube.CoreV1().ServiceAccounts(obj.Namespace).Create(obj)
		return err
	case *core.Service:
		_, err := kube.CoreV1().Services(obj.Namespace).Create(obj)
		return err
	case *rbac.ClusterRole:
		_, err := kube.RbacV1().ClusterRoles().Create(obj)
		return err
	case *rbac.ClusterRoleBinding:
		_, err := kube.RbacV1().ClusterRoleBindings().Create(obj)
		return err
	case *batch.Job:
		_, err := kube.BatchV1().Jobs(obj.Namespace).Create(obj)
		return err
	case *appsv1beta2.Deployment:
		_, err := kube.AppsV1beta2().Deployments(obj.Namespace).Create(obj)
		return err
	case *appsv1beta2.DaemonSet:
		_, err := kube.AppsV1beta2().DaemonSets(obj.Namespace).Create(obj)
		return err
	case *apiextensions.CustomResourceDefinition:
		_, err := exts.ApiextensionsV1beta1().CustomResourceDefinitions().Create(obj)
		return err
	case *v1beta1.MutatingWebhookConfiguration:
		_, err := kube.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Create(obj)
		return err
	case *autoscaling.HorizontalPodAutoscaler:
		_, err := kube.AutoscalingV1().HorizontalPodAutoscalers(obj.Namespace).Create(obj)
		return err
	}
	return errors.Errorf("no implementation for type %v", obj)
}

// resource version should be ignored / not matter
func (k *kubeInterface) update(obj runtime.Object) error {
	kube := k.kube
	exts := k.exts
	switch obj := obj.(type) {
	case *core.Namespace:
		client := kube.CoreV1().Namespaces()
		obj2, err := client.Get(obj.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.ResourceVersion = obj2.ResourceVersion
		_, err = client.Update(obj)
		return err
	case *core.ConfigMap:
		client := kube.CoreV1().ConfigMaps(obj.Namespace)
		obj2, err := client.Get(obj.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.ResourceVersion = obj2.ResourceVersion
		_, err = client.Update(obj)
		return err
	case *core.ServiceAccount:
		client := kube.CoreV1().ServiceAccounts(obj.Namespace)
		obj2, err := client.Get(obj.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.ResourceVersion = obj2.ResourceVersion
		_, err = client.Update(obj)
		return err
	case *core.Service:
		client := kube.CoreV1().Services(obj.Namespace)
		obj2, err := client.Get(obj.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.ResourceVersion = obj2.ResourceVersion
		_, err = client.Update(obj)
		return err
	case *rbac.ClusterRole:
		client := kube.RbacV1().ClusterRoles()
		obj2, err := client.Get(obj.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.ResourceVersion = obj2.ResourceVersion
		_, err = client.Update(obj)
		return err
	case *rbac.ClusterRoleBinding:
		client := kube.RbacV1().ClusterRoleBindings()
		obj2, err := client.Get(obj.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.ResourceVersion = obj2.ResourceVersion
		_, err = client.Update(obj)
		return err
	case *batch.Job:
		client := kube.BatchV1().Jobs(obj.Namespace)
		obj2, err := client.Get(obj.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.ResourceVersion = obj2.ResourceVersion
		_, err = client.Update(obj)
		return err
	case *appsv1beta2.Deployment:
		client := kube.AppsV1beta2().Deployments(obj.Namespace)
		obj2, err := client.Get(obj.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.ResourceVersion = obj2.ResourceVersion
		_, err = client.Update(obj)
		return err
	case *appsv1beta2.DaemonSet:
		client := kube.AppsV1beta2().DaemonSets(obj.Namespace)
		obj2, err := client.Get(obj.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.ResourceVersion = obj2.ResourceVersion
		_, err = client.Update(obj)
		return err
	case *apiextensions.CustomResourceDefinition:
		client := exts.ApiextensionsV1beta1().CustomResourceDefinitions()
		obj2, err := client.Get(obj.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.ResourceVersion = obj2.ResourceVersion
		_, err = client.Update(obj)
		return err
	case *v1beta1.MutatingWebhookConfiguration:
		client := kube.AdmissionregistrationV1beta1().MutatingWebhookConfigurations()
		obj2, err := client.Get(obj.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.ResourceVersion = obj2.ResourceVersion
		_, err = client.Update(obj)
		return err
	case *autoscaling.HorizontalPodAutoscaler:
		client := kube.AutoscalingV1().HorizontalPodAutoscalers(obj.Namespace)
		obj2, err := client.Get(obj.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		obj.ResourceVersion = obj2.ResourceVersion
		_, err = client.Update(obj)
		return err
	}
	return errors.Errorf("no implementation for type %v", obj)
}

// this can be just an empty object of the correct type w/ the name and namespace (if applicable) set
func (k *kubeInterface) delete(obj runtime.Object) error {
	kube := k.kube
	exts := k.exts
	switch obj := obj.(type) {
	case *core.Namespace:
		return kube.CoreV1().Namespaces().Delete(obj.Name, nil)
	case *core.ConfigMap:
		return kube.CoreV1().ConfigMaps(obj.Namespace).Delete(obj.Name, nil)
	case *core.ServiceAccount:
		return kube.CoreV1().ServiceAccounts(obj.Namespace).Delete(obj.Name, nil)
	case *core.Service:
		return kube.CoreV1().Services(obj.Namespace).Delete(obj.Name, nil)
	case *rbac.ClusterRole:
		return kube.RbacV1().ClusterRoles().Delete(obj.Name, nil)
	case *rbac.ClusterRoleBinding:
		return kube.RbacV1().ClusterRoleBindings().Delete(obj.Name, nil)
	case *batch.Job:
		return kube.BatchV1().Jobs(obj.Namespace).Delete(obj.Name, nil)
	case *appsv1beta2.Deployment:
		return kube.AppsV1beta2().Deployments(obj.Namespace).Delete(obj.Name, nil)
	case *appsv1beta2.DaemonSet:
		return kube.AppsV1beta2().DaemonSets(obj.Namespace).Delete(obj.Name, nil)
	case *apiextensions.CustomResourceDefinition:
		return exts.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(obj.Name, nil)
	case *v1beta1.MutatingWebhookConfiguration:
		return kube.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Delete(obj.Name, nil)
	case *autoscaling.HorizontalPodAutoscaler:
		return kube.AutoscalingV1().HorizontalPodAutoscalers(obj.Namespace).Delete(obj.Name, nil)
	}
	return errors.Errorf("no implementation for type %v", obj)
}
