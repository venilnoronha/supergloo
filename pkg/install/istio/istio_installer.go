package istio

import (
	"github.com/pkg/errors"
	"github.com/solo-io/supergloo/pkg/install/shared"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"strconv"
	"strings"

	security "github.com/openshift/client-go/security/clientset/versioned"
	"github.com/solo-io/supergloo/pkg/api/v1"
	apiexts "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	kubemeta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	CrbName          = "istio-crb"
	defaultNamespace = "istio-system"
)

type IstioInstaller struct {
	apiExts        apiexts.Interface
	securityClient *security.Clientset
	crds           []*v1beta1.CustomResourceDefinition
}

func NewIstioInstaller(ApiExts apiexts.Interface, SecurityClient *security.Clientset) (*IstioInstaller, error) {
	crds, err := shared.CrdsFromManifest(IstioCrdYaml)
	if err != nil {
		return nil, err
	}
	return &IstioInstaller{
		apiExts:        ApiExts,
		securityClient: SecurityClient,
		crds:           crds,
	}, nil
}

func (c *IstioInstaller) GetDefaultNamespace() string {
	return defaultNamespace
}

func (c *IstioInstaller) GetCrbName() string {
	return CrbName
}

func (c *IstioInstaller) GetOverridesYaml(install *v1.Install) string {
	return getOverrides(install.Encryption)
}

func getOverrides(encryption *v1.Encryption) string {
	selfSigned := false
	mtlsEnabled := false
	if encryption != nil {
		if encryption.TlsEnabled {
			mtlsEnabled = true
			if encryption.Secret != nil {
				selfSigned = true
			}
		}
	}
	selfSignedString := strconv.FormatBool(selfSigned)
	tlsEnabledString := strconv.FormatBool(mtlsEnabled)
	overridesWithMtlsFlag := strings.Replace(overridesYaml, "@@MTLS_ENABLED@@", tlsEnabledString, -1)
	return strings.Replace(overridesWithMtlsFlag, "@@SELF_SIGNED@@", selfSignedString, -1)
}

var overridesYaml = `
global.mtls.enabled: @@MTLS_ENABLED@@
security.selfSigned: @@SELF_SIGNED@@
global.crds: false
`

func (c *IstioInstaller) DoPostHelmInstall(install *v1.Install, kube *kubernetes.Clientset, releaseName string) error {
	return nil
}

func (c *IstioInstaller) DoPreHelmInstall() error {
	// create crds if they don't exist. CreateCrds does not error on err type IsAlreadyExists
	if err := shared.CreateCrds(c.apiExts, c.crds...); err != nil {
		return errors.Wrapf(err, "creating istio crds")
	}
	if c.securityClient == nil {
		return nil
	}
	return c.AddSccToUsers(
		"default",
		"istio-ingress-service-account",
		"prometheus",
		"istio-egressgateway-service-account",
		"istio-citadel-service-account",
		"istio-ingressgateway-service-account",
		"istio-cleanup-old-ca-service-account",
		"istio-mixer-post-install-account",
		"istio-mixer-service-account",
		"istio-pilot-service-account",
		"istio-sidecar-injector-service-account",
		"istio-galley-service-account")
}

// TODO: something like this should enable minishift installs to succeed, but this isn't right. The correct steps are
//       to run "oc adm policy add-scc-to-user anyuid -z %s -n istio-system" for each of the user accounts above
//       maybe the issue is not specifying the namespace?
func (c *IstioInstaller) AddSccToUsers(users ...string) error {
	anyuid, err := c.securityClient.SecurityV1().SecurityContextConstraints().Get("anyuid", kubemeta.GetOptions{})
	if err != nil {
		return err
	}
	newUsers := append(anyuid.Users, users...)
	anyuid.Users = newUsers
	_, err = c.securityClient.SecurityV1().SecurityContextConstraints().Update(anyuid)
	return err
}

func (c *IstioInstaller) DoPreHelmUninstall() error {
	return nil
}

func (c *IstioInstaller) DoPostHelmUninstall() error {
	return nil
}
