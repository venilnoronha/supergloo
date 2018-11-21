package e2e

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/solo-io/supergloo/pkg/install/istio"
	kubecore "k8s.io/api/core/v1"
	kubemeta "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/supergloo/pkg/install"

	istiosecret "github.com/solo-io/supergloo/pkg/api/external/istio/encryption/v1"
	"github.com/solo-io/supergloo/test/util"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	gloo "github.com/solo-io/supergloo/pkg/api/external/gloo/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/supergloo/pkg/api/v1"
	istioSync "github.com/solo-io/supergloo/pkg/translator/istio"
)

/*
End to end tests for istio install and mesh syncing.
*/
var _ = Describe("Istio Install and Encryption E2E", func() {
	defer GinkgoRecover()

	installNamespace := "istio-system"
	superglooNamespace := "supergloo-system" // this needs to be made before running tests
	meshName := "test-istio-mesh"
	secretName := "test-tls-secret"
	kubeCache := kube.NewKubeCache()
	path := os.Getenv("HELM_CHART_PATH")
	if path == "" {
		path = "https://s3.amazonaws.com/supergloo.solo.io/istio-1.0.3.tgz"
	}

	getSnapshot := func(mtls bool, secret *core.ResourceRef) *v1.InstallSnapshot {
		return &v1.InstallSnapshot{
			Installs: v1.InstallsByNamespace{
				installNamespace: v1.InstallList{
					&v1.Install{
						Metadata: core.Metadata{
							Namespace: superglooNamespace,
							Name:      meshName,
						},
						MeshType: &v1.Install_Istio{
							Istio: &v1.Istio{
								InstallationNamespace: installNamespace,
							},
						},
						ChartLocator: &v1.HelmChartLocator{
							Kind: &v1.HelmChartLocator_ChartPath{
								ChartPath: &v1.HelmChartPath{
									Path: path,
								},
							},
						},
						Encryption: &v1.Encryption{
							TlsEnabled: mtls,
							Secret:     secret,
						},
					},
				},
			},
		}
	}

	getTranslatorSnapshot := func(mesh *v1.Mesh, secret *istiosecret.IstioCacertsSecret) *v1.TranslatorSnapshot {
		secrets := istiosecret.IstiocertsByNamespace{}
		if secret != nil {
			secrets = istiosecret.IstiocertsByNamespace{
				superglooNamespace: istiosecret.IstioCacertsSecretList{
					secret,
				},
			}
		}
		return &v1.TranslatorSnapshot{
			Meshes: v1.MeshesByNamespace{
				superglooNamespace: v1.MeshList{
					mesh,
				},
			},
			Istiocerts: secrets,
		}
	}

	var meshClient v1.MeshClient
	var upstreamClient gloo.UpstreamClient

	var secretClient istiosecret.IstioCacertsSecretClient
	var installSyncer install.InstallSyncer

	BeforeEach(func() {
		util.TryCreateNamespace("supergloo-system")
		util.TryCreateNamespace("gloo-system")
		meshClient = util.GetMeshClient(kubeCache)
		upstreamClient = util.GetUpstreamClient(kubeCache)

		secretClient = util.GetSecretClient()
		installSyncer = install.InstallSyncer{
			Kube:       util.GetKubeClient(),
			MeshClient: meshClient,
		}
	})

	AfterEach(func() {
		util.UninstallHelmRelease(meshName)
		util.TryDeleteIstioCrds()
		util.TerminateNamespaceBlocking(installNamespace)
		util.DeleteCrb(istio.CrbName)
		if meshClient != nil {
			meshClient.Delete(superglooNamespace, meshName, clients.DeleteOpts{})
		}
		if secretClient != nil {
			secretClient.Delete(superglooNamespace, secretName, clients.DeleteOpts{})
			secretClient.Delete(installNamespace, istioSync.CustomRootCertificateSecretName, clients.DeleteOpts{})
		}
		util.TerminateNamespaceBlocking("supergloo-system")
		// delete gloo system to remove gloo resources like upstreams
		util.TerminateNamespaceBlocking("gloo-system")
	})

	It("Can install istio with mtls enabled and custom root cert", func() {
		secret, ref := util.CreateTestSecret(superglooNamespace, secretName)
		snap := getSnapshot(true, ref)
		err := installSyncer.Sync(context.TODO(), snap)
		Expect(err).NotTo(HaveOccurred())

		util.WaitForAvailablePods(installNamespace)
		mesh, err := meshClient.Read(superglooNamespace, meshName, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())

		meshSyncer := istioSync.EncryptionSyncer{
			Kube:           util.GetKubeClient(),
			SecretClient:   secretClient,
			IstioNamespace: installNamespace,
		}
		syncSnapshot := getTranslatorSnapshot(mesh, secret)
		err = meshSyncer.Sync(context.TODO(), syncSnapshot)
		Expect(err).NotTo(HaveOccurred())

		util.CheckCertMatchesIstio(installNamespace)
		// TODO: add more checking:
		// - istio.default has actually been deleted and not regenerating
		// - new certs are signed correctly
	})

	It("Can install istio without mtls enabled", func() {
		snap := getSnapshot(false, nil)
		err := installSyncer.Sync(context.TODO(), snap)
		Expect(err).NotTo(HaveOccurred())
		util.WaitForAvailablePods(installNamespace)

		mesh, err := meshClient.Read(superglooNamespace, meshName, clients.ReadOpts{})
		Expect(err).NotTo(HaveOccurred())
		meshSyncer := istioSync.EncryptionSyncer{}
		syncSnapshot := getTranslatorSnapshot(mesh, nil)
		err = meshSyncer.Sync(context.TODO(), syncSnapshot)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("istio + policy", func() {

		var (
			bookinfons string
		)
		BeforeEach(func() {
			// TODO: change this to something random once we fix discovery
			// to work with labeled namespaces
			bookinfons = "default"
		})

		AfterEach(func() {
			gexec.TerminateAndWait(2 * time.Second)
			if bookinfons != "default" {
				util.TerminateNamespaceBlocking(bookinfons)
			}
		})

		deployBookInfo := func() string {
			// create namespace for bookinfo
			ns := &kubecore.Namespace{
				ObjectMeta: kubemeta.ObjectMeta{
					Name: bookinfons,
					Labels: map[string]string{
						"istio-injection": "enabled",
					},
				},
			}
			util.GetKubeClient().CoreV1().Namespaces().Create(ns)

			bookinfo := "https://raw.githubusercontent.com/istio/istio/4c0a001b5e542d43b4c66ae75c1f41f2c1ff183e/samples/bookinfo/platform/kube/bookinfo.yaml"
			kubectlargs := []string{"apply", "-n", bookinfons, "-f", bookinfo}
			cmd := exec.Command("kubectl", kubectlargs...)
			cmd.Stdout = GinkgoWriter
			cmd.Stderr = GinkgoWriter
			err := cmd.Run()
			Expect(err).NotTo(HaveOccurred())

			return bookinfons
		}

		It("Should install istio and enable policy", func() {

			// start discovery
			cmd := exec.Command(PathToUds, "-udsonly")
			_, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)

			snap := getSnapshot(true, nil)
			err = installSyncer.Sync(context.TODO(), snap)
			Expect(err).NotTo(HaveOccurred())
			util.WaitForAvailablePodsWithTimeout(installNamespace, "300s")

			deployBookInfo()
			util.WaitForAvailablePodsWithTimeout(bookinfons, "500s")

			mesh, err := meshClient.Read(superglooNamespace, meshName, clients.ReadOpts{})
			Expect(err).NotTo(HaveOccurred())

			mesh.Policy = &v1.Policy{
				Rules: []*v1.Rule{
					{
						Source: &core.ResourceRef{
							Name:      "default-reviews-9080",
							Namespace: "gloo-system",
						},
						Destination: &core.ResourceRef{
							Name:      "default-ratings-9080",
							Namespace: "gloo-system",
						},
					},
				},
			}

			meshSyncer, err := istioSync.NewPolicySyncer("supergloo-system", kubeCache, util.GetKubeConfig())
			Expect(err).NotTo(HaveOccurred())

			getupstreamnames := func() ([]string, error) {
				return util.GetUpstreamNames(upstreamClient)
			}
			Eventually(getupstreamnames, "60s", "1s").ShouldNot(HaveLen(0))

			syncSnapshot := getTranslatorSnapshot(mesh, nil)
			ups, err := upstreamClient.List("gloo-system", clients.ListOpts{})
			Expect(err).NotTo(HaveOccurred())
			syncSnapshot.Upstreams = gloo.UpstreamsByNamespace{
				"gloo-system": ups,
			}

			err = meshSyncer.Sync(context.TODO(), syncSnapshot)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
