package install

import (
	"github.com/solo-io/supergloo/cli/pkg/cmd/options"
	"github.com/solo-io/supergloo/pkg/api/v1"
)

func generateConsulInstallSpecFromOpts(opts *options.Options) *v1.Install {
	installSpec := &v1.Install{
		Metadata: getMetadataFromOpts(opts),
		MeshType: &v1.Install_Consul{
			Consul: &v1.Consul{
				InstallationNamespace: opts.Install.Namespace,
				ServerAddress:         opts.Install.ConsulServerAddress,
			},
		},
	}
	installSpec.Encryption = getEncryptionFromOpts(opts)
	return installSpec
}

func generateIstioInstallSpecFromOpts(opts *options.Options) *v1.Install {
	installSpec := &v1.Install{
		Metadata: getMetadataFromOpts(opts),
		MeshType: &v1.Install_Istio{
			Istio: &v1.Istio{
				InstallationNamespace: opts.Install.Namespace,
				WatchNamespaces:       opts.Install.WatchNamespaces,
			},
		},
	}
	installSpec.Encryption = getEncryptionFromOpts(opts)
	return installSpec
}

func generateLinkerd2InstallSpecFromOpts(opts *options.Options) *v1.Install {
	installSpec := &v1.Install{
		Metadata: getMetadataFromOpts(opts),
		MeshType: &v1.Install_Linkerd2{
			Linkerd2: &v1.Linkerd2{
				InstallationNamespace: opts.Install.Namespace,
				WatchNamespaces:       opts.Install.WatchNamespaces,
			},
		},
	}
	installSpec.Encryption = getEncryptionFromOpts(opts)
	return installSpec
}
