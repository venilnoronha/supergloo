package consul_test

import (
	"context"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/supergloo/test/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/supergloo/pkg/api/v1"
	"github.com/solo-io/supergloo/pkg/install/consul"
)

/*
End to end tests for consul installs with and without mTLS enabled.
Tests assume you already have a Kubernetes environment with Helm / Tiller set up, and with a "supergloo-system" namespace.
The tests will install Consul and get it configured and validate all services up and running, then tear down and
clean up all resources created. This will take about 45 seconds with mTLS, and 20 seconds without.
*/
var _ = Describe("ConsulInstallSyncer", func() {

	installNamespace := "consul"
	superglooNamespace := "supergloo-system" // this needs to be made before running tests
	meshName := "test-consul-mesh"

	getSnapshot := func(mtls bool) *v1.InstallSnapshot {
		return &v1.InstallSnapshot{
			Installs: v1.InstallsByNamespace{
				superglooNamespace: v1.InstallList{
					&v1.Install{
						Metadata: core.Metadata{
							Namespace: superglooNamespace,
							Name:      meshName,
						},
						Consul: &v1.ConsulInstall{
							Path:      "https://github.com/hashicorp/consul-helm/archive/v0.3.0.tar.gz",
							Namespace: installNamespace,
						},
						Encryption: &v1.Encryption{
							TlsEnabled: mtls,
						},
					},
				},
			},
		}
	}

	kubeCache := kube.NewKubeCache()

	var meshClient v1.MeshClient
	var syncer consul.ConsulInstallSyncer

	BeforeEach(func() {
		meshClient = util.GetMeshClient(kubeCache)
		syncer = consul.ConsulInstallSyncer{
			Kube:       util.GetKubeClient(),
			MeshClient: meshClient,
		}
	})

	AfterEach(func() {
		util.DeleteWebhookConfigIfExists(consul.WebhookCfg)
		util.DeleteCrb(consul.CrbName)
		util.TerminateNamespaceBlocking(installNamespace)
		meshClient.Delete(superglooNamespace, meshName, clients.DeleteOpts{})
	})

	It("Can install consul with mtls enabled", func() {
		snap := getSnapshot(true)
		err := syncer.Sync(context.TODO(), snap)
		Expect(err).NotTo(HaveOccurred())
		util.WaitForAvailablePods(installNamespace)
	})

	It("Can install consul without mtls enabled", func() {
		snap := getSnapshot(false)
		err := syncer.Sync(context.TODO(), snap)
		Expect(err).NotTo(HaveOccurred())
		util.WaitForAvailablePods(installNamespace)
	})
})