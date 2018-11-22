package install

import (
	"context"

	"github.com/solo-io/solo-kit/pkg/utils/contextutils"

	apiexts "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"github.com/solo-io/supergloo/pkg/install/linkerd2"

	"github.com/solo-io/supergloo/pkg/install/istio"

	"github.com/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/supergloo/pkg/api/v1"
	"github.com/solo-io/supergloo/pkg/install/consul"
	"github.com/solo-io/supergloo/pkg/install/helm"
	"k8s.io/client-go/kubernetes"

	security "github.com/openshift/client-go/security/clientset/versioned"
	kubecore "k8s.io/api/core/v1"
	kuberbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kubemeta "k8s.io/apimachinery/pkg/apis/meta/v1"
	helmlib "k8s.io/helm/pkg/helm"
)

const releaseNameKey = "helm_release"

type InstallSyncer struct {
	Kube           *kubernetes.Clientset
	MeshClient     v1.MeshClient
	SecurityClient *security.Clientset
	ApiExts        apiexts.Interface
}

type MeshInstaller interface {
	GetDefaultNamespace() string
	GetCrbName() string
	GetOverridesYaml(install *v1.Install) string
	DoPreHelmInstall() error
	DoPostHelmInstall(install *v1.Install, kube *kubernetes.Clientset, releaseName string) error
	DoPreHelmUninstall() error
	DoPostHelmUninstall() error
}

func (syncer *InstallSyncer) Sync(ctx context.Context, snap *v1.InstallSnapshot) error {
	ctx = contextutils.WithLogger(ctx, "install-syncer")
	for _, install := range snap.Installs.List() {
		err := syncer.syncInstall(ctx, install)
		if err != nil {
			return err
		}
	}
	return nil
}

func (syncer *InstallSyncer) syncInstall(ctx context.Context, install *v1.Install) error {
	var meshInstaller MeshInstaller
	switch install.MeshType.(type) {
	case *v1.Install_Consul:
		meshInstaller = &consul.ConsulInstaller{}
	case *v1.Install_Istio:
		meshInstaller = &istio.IstioInstaller{
			ApiExts: syncer.ApiExts,
			SecurityClient: syncer.SecurityClient,
		}
	case *v1.Install_Linkerd2:
		meshInstaller = &linkerd2.Linkerd2Installer{}
	default:
		return errors.Errorf("Unsupported mesh type %v", install.MeshType)
	}

	installEnabled := install.Enabled == nil || install.Enabled.Value

	mesh, meshErr := syncer.MeshClient.Read(install.Metadata.Namespace, install.Metadata.Name, clients.ReadOpts{Ctx: ctx})
	switch {
	case meshErr == nil && !installEnabled:
		if err := meshInstaller.DoPreHelmUninstall(); err != nil {
			return err
		}
		releaseName := mesh.Metadata.Annotations[releaseNameKey]
		if err := uninstallHelmRelease(ctx, releaseName); err != nil {
			return err
		}
		if err := meshInstaller.DoPostHelmUninstall(); err != nil {
			return err
		}
		return syncer.MeshClient.Delete(mesh.Metadata.Namespace, mesh.Metadata.Name, clients.DeleteOpts{Ctx: ctx})
	case meshErr != nil && installEnabled:
		releaseName, err := syncer.installHelmRelease(ctx, install, meshInstaller)
		if err != nil {
			return err
		}
		return syncer.createMesh(ctx, install, releaseName)
	}
	return nil
}

func (syncer *InstallSyncer) installHelmRelease(ctx context.Context, install *v1.Install, installer MeshInstaller) (string, error) {
	contextutils.LoggerFrom(ctx).Infof("setting up namespace")
	// 1. Setup namespace
	installNamespace, err := syncer.SetupInstallNamespace(install, installer.GetDefaultNamespace())
	if err != nil {
		return "", err
	}

	// 2. Set up ClusterRoleBinding for that namespace
	// This is not cleaned up when deleting namespace so it may already exist on the system, don't fail
	crbName := installer.GetCrbName()
	if crbName != "" {
		err = syncer.CreateCrbIfNotExist(crbName, installNamespace)
		if err != nil {
			return "", err
		}
	}

	contextutils.LoggerFrom(ctx).Infof("helm pre-install")
	// 3. Do any pre-helm tasks
	err = installer.DoPreHelmInstall()
	if err != nil {
		return "", errors.Wrap(err, "Error doing pre-helm install steps")
	}

	contextutils.LoggerFrom(ctx).Infof("helm install")
	// 4. Install mesh via helm chart
	releaseName, err := syncer.HelmInstall(ctx, install.ChartLocator, install.Metadata.Name, installNamespace, installer.GetOverridesYaml(install))
	if err != nil {
		return "", errors.Wrap(err, "Error installing helm chart")
	}

	contextutils.LoggerFrom(ctx).Infof("installed %v", releaseName)
	// 5. Do any additional steps
	return releaseName, installer.DoPostHelmInstall(install, syncer.Kube, releaseName)
}

func (syncer *InstallSyncer) SetupInstallNamespace(install *v1.Install, defaultNamespace string) (string, error) {
	installNamespace := getInstallNamespace(install, defaultNamespace)
	err := syncer.createNamespaceIfNotExist(installNamespace) // extract to CRD
	if err != nil {
		return installNamespace, errors.Wrap(err, "Error setting up namespace")
	}
	return installNamespace, nil
}

func getInstallNamespace(install *v1.Install, defaultNamespace string) string {
	installNamespace := getInstallationNamespace(install)
	if installNamespace != "" {
		return installNamespace
	}
	return defaultNamespace
}

func getInstallationNamespace(install *v1.Install) (installationNamespace string) {
	switch x := install.MeshType.(type) {
	case *v1.Install_Istio:
		return x.Istio.InstallationNamespace
	case *v1.Install_Consul:
		return x.Consul.InstallationNamespace
	case *v1.Install_Linkerd2:
		return x.Linkerd2.InstallationNamespace
	default:
		//should never happen
		return ""
	}
}

func (syncer *InstallSyncer) createNamespaceIfNotExist(namespaceName string) error {
	_, err := syncer.Kube.CoreV1().Namespaces().Create(getNamespace(namespaceName))
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func getNamespace(namespaceName string) *kubecore.Namespace {
	return &kubecore.Namespace{
		ObjectMeta: kubemeta.ObjectMeta{
			Name: namespaceName,
		},
	}
}

func (syncer *InstallSyncer) CreateCrbIfNotExist(crbName string, namespaceName string) error {
	_, err := syncer.Kube.RbacV1().ClusterRoleBindings().Create(getCrb(crbName, namespaceName))
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func getCrb(crbName string, namespaceName string) *kuberbac.ClusterRoleBinding {
	meta := kubemeta.ObjectMeta{
		Name: crbName,
	}
	subject := kuberbac.Subject{
		Kind:      "ServiceAccount",
		Namespace: namespaceName,
		Name:      "default",
	}
	roleRef := kuberbac.RoleRef{
		Kind:     "ClusterRole",
		Name:     "cluster-admin",
		APIGroup: "rbac.authorization.k8s.io",
	}
	return &kuberbac.ClusterRoleBinding{
		ObjectMeta: meta,
		Subjects:   []kuberbac.Subject{subject},
		RoleRef:    roleRef,
	}
}

func (syncer *InstallSyncer) HelmInstall(ctx context.Context, chartLocator *v1.HelmChartLocator, releaseName string, installNamespace string, overridesYaml string) (string, error) {
	if chartLocator.GetChartPath() != nil {
		return helmInstallChart(ctx, chartLocator.GetChartPath().Path, releaseName, installNamespace, overridesYaml)
	}
	return "", errors.Errorf("Unsupported kind of chart locator")
}

func helmInstallChart(ctx context.Context, chartPath string, releaseName string, installNamespace string, overridesYaml string) (string, error) {
	// helm install
	helmClient, err := helm.GetHelmClient(ctx)
	if err != nil {
		return "", err
	}

	installPath, err := helm.LocateChartRepoReleaseDefault(ctx, "", chartPath)
	if err != nil {
		return "", err
	}
	response, err := helmClient.InstallRelease(
		installPath,
		installNamespace,
		helmlib.ValueOverrides([]byte(overridesYaml)),
		helmlib.ReleaseName(releaseName))
	helm.Teardown()
	if err != nil {
		return "", err
	}
	return response.Release.Name, nil
}

func (syncer *InstallSyncer) createMesh(ctx context.Context, install *v1.Install, releaseName string) error {
	mesh, err := getMeshObject(install, releaseName)
	if err != nil {
		return err
	}
	_, err = syncer.MeshClient.Write(mesh, clients.WriteOpts{Ctx: ctx})
	return err
}

func getMeshObject(install *v1.Install, releaseName string) (*v1.Mesh, error) {
	mesh := &v1.Mesh{
		Metadata: core.Metadata{
			Name:        install.Metadata.Name,
			Namespace:   install.Metadata.Namespace,
			Annotations: map[string]string{releaseNameKey: releaseName},
		},
		Encryption: install.Encryption,
	}
	var err error
	switch x := install.MeshType.(type) {
	case *v1.Install_Istio:
		mesh.MeshType = &v1.Mesh_Istio{
			Istio: x.Istio,
		}
	case *v1.Install_Consul:
		mesh.MeshType = &v1.Mesh_Consul{
			Consul: x.Consul,
		}
	case *v1.Install_Linkerd2:
		mesh.MeshType = &v1.Mesh_Linkerd2{
			Linkerd2: x.Linkerd2,
		}
	default:
		err = errors.Errorf("Unsupported mesh type.")
	}
	return mesh, err
}

func uninstallHelmRelease(ctx context.Context, releaseName string) error {
	helmClient, err := helm.GetHelmClient(ctx)
	if err != nil {
		return err
	}
	_, err = helmClient.DeleteRelease(releaseName, helmlib.DeletePurge(true))
	helm.Teardown()
	return err
}
