package addonfactory

import (
	"embed"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"open-cluster-management.io/addon-framework/pkg/agent"
)

const AddonDefaultInstallNamespace = "open-cluster-management-agent-addon"

// AnnotationValuesName is the annotation Name of customized values
const AnnotationValuesName string = "addon.open-cluster-management.io/values"

type Values map[string]interface{}

type GetValuesFunc func(cluster *clusterv1.ManagedCluster,
	addon *addonapiv1alpha1.ManagedClusterAddOn) (Values, error)

// AgentAddonFactory includes the common fields for building different agentAddon instances.
type AgentAddonFactory struct {
	scheme            *runtime.Scheme
	fs                embed.FS
	dir               string
	getValuesFuncs    []GetValuesFunc
	agentAddonOptions agent.AgentAddonOptions
	// trimCRDDescription flag is used to trim the description of CRDs in manifestWork. disabled by default.
	trimCRDDescription bool
	hostingCluster     *clusterv1.ManagedCluster
}

// NewAgentAddonFactory builds an addonAgentFactory instance with addon name and fs.
// dir is the path prefix based on the fs path.
func NewAgentAddonFactory(addonName string, fs embed.FS, dir string) *AgentAddonFactory {
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = apiextensionsv1.AddToScheme(s)
	_ = apiextensionsv1beta1.AddToScheme(s)

	return &AgentAddonFactory{
		fs:  fs,
		dir: dir,
		agentAddonOptions: agent.AgentAddonOptions{
			AddonName:           addonName,
			Registration:        nil,
			InstallStrategy:     nil,
			HealthProber:        nil,
			SupportedConfigGVRs: []schema.GroupVersionResource{},
		},
		trimCRDDescription: false,
		scheme:             s,
	}
}

// WithScheme is an optional configuration, only used when the agentAddon has customized resource types.
func (f *AgentAddonFactory) WithScheme(s *runtime.Scheme) *AgentAddonFactory {
	f.scheme = s
	_ = scheme.AddToScheme(f.scheme)
	_ = apiextensionsv1.AddToScheme(f.scheme)
	_ = apiextensionsv1beta1.AddToScheme(f.scheme)
	return f
}

// WithGetValuesFuncs adds a list of the getValues func.
// the values got from the big index Func will override the one from small index Func.
func (f *AgentAddonFactory) WithGetValuesFuncs(getValuesFuncs ...GetValuesFunc) *AgentAddonFactory {
	f.getValuesFuncs = getValuesFuncs
	return f
}

// WithInstallStrategy defines the installation strategy of the manifests prescribed by Manifests(..).
// Deprecated: add annotation "addon.open-cluster-management.io/lifecycle: addon-manager" to ClusterManagementAddon
// and define install strategy in ClusterManagementAddon spec.installStrategy instead.
// The migration plan refer to https://github.com/open-cluster-management-io/ocm/issues/355.
func (f *AgentAddonFactory) WithInstallStrategy(strategy *agent.InstallStrategy) *AgentAddonFactory {
	if strategy.InstallNamespace == "" {
		strategy.InstallNamespace = AddonDefaultInstallNamespace
	}
	f.agentAddonOptions.InstallStrategy = strategy

	return f
}

// WithAgentRegistrationOption defines how agent is registered to the hub cluster.
func (f *AgentAddonFactory) WithAgentRegistrationOption(option *agent.RegistrationOption) *AgentAddonFactory {
	f.agentAddonOptions.Registration = option
	return f
}

// WithAgentHealthProber defines how is the healthiness status of the ManagedClusterAddon probed.
func (f *AgentAddonFactory) WithAgentHealthProber(prober *agent.HealthProber) *AgentAddonFactory {
	f.agentAddonOptions.HealthProber = prober
	return f
}

// WithAgentHostedModeEnabledOption will enable the agent hosted deploying mode.
func (f *AgentAddonFactory) WithAgentHostedModeEnabledOption() *AgentAddonFactory {
	f.agentAddonOptions.HostedModeEnabled = true
	return f
}

// WithTrimCRDDescription is to enable trim the description of CRDs in manifestWork.
func (f *AgentAddonFactory) WithTrimCRDDescription() *AgentAddonFactory {
	f.trimCRDDescription = true
	return f
}

// WithConfigGVRs defines the addon supported configuration GroupVersionResource
func (f *AgentAddonFactory) WithConfigGVRs(gvrs ...schema.GroupVersionResource) *AgentAddonFactory {
	f.agentAddonOptions.SupportedConfigGVRs = append(f.agentAddonOptions.SupportedConfigGVRs, gvrs...)
	return f
}

// WithHostingCluster defines the hosting cluster used in hosted mode. An AgentAddon may use this to provide
// additional metadata.
func (f *AgentAddonFactory) WithHostingCluster(cluster *clusterv1.ManagedCluster) *AgentAddonFactory {
	f.hostingCluster = cluster
	return f
}

// WithAgentDeployTriggerClusterFilter defines the filter func to trigger the agent deploy/redploy when cluster info is
// changed. Addons that need information from the ManagedCluster resource when deploying the agent should use this
// function to set what information they need, otherwise the expected/up-to-date agent may be deployed delayed since the
// default filter func returns false when the ManagedCluster resource is updated.
//
// For example, the agentAddon needs information from the ManagedCluster annotation, it can set the filter function
// like:
//
//	WithAgentDeployClusterTriggerFilter(func(old, new *clusterv1.ManagedCluster) bool {
//	 return !equality.Semantic.DeepEqual(old.Annotations, new.Annotations)
//	})
func (f *AgentAddonFactory) WithAgentDeployTriggerClusterFilter(
	filter func(old, new *clusterv1.ManagedCluster) bool,
) *AgentAddonFactory {
	f.agentAddonOptions.AgentDeployTriggerClusterFilter = filter
	return f
}

// BuildHelmAgentAddon builds a helm agentAddon instance.
func (f *AgentAddonFactory) BuildHelmAgentAddon() (agent.AgentAddon, error) {
	if err := validateSupportedConfigGVRs(f.agentAddonOptions.SupportedConfigGVRs); err != nil {
		return nil, err
	}

	userChart, err := loadChart(f.fs, f.dir)
	if err != nil {
		return nil, err
	}

	agentAddon := newHelmAgentAddon(f, userChart)

	return agentAddon, nil
}

// BuildTemplateAgentAddon builds a template agentAddon instance.
func (f *AgentAddonFactory) BuildTemplateAgentAddon() (agent.AgentAddon, error) {
	if err := validateSupportedConfigGVRs(f.agentAddonOptions.SupportedConfigGVRs); err != nil {
		return nil, err
	}

	templateFiles, err := getTemplateFiles(f.fs, f.dir)
	if err != nil {
		klog.Errorf("failed to get template files. %v", err)
		return nil, err
	}
	if len(templateFiles) == 0 {
		return nil, fmt.Errorf("there is no template files")
	}

	agentAddon := newTemplateAgentAddon(f)

	for _, file := range templateFiles {
		template, err := f.fs.ReadFile(file)
		if err != nil {
			return nil, err
		}
		agentAddon.addTemplateData(file, template)
	}
	return agentAddon, nil
}

func validateSupportedConfigGVRs(configGVRs []schema.GroupVersionResource) error {
	if len(configGVRs) == 0 {
		// no configs required, ignore
		return nil
	}

	configGVRMap := map[schema.GroupVersionResource]bool{}
	for index, gvr := range configGVRs {
		if gvr.Empty() {
			return fmt.Errorf("config type is empty, index=%d", index)
		}

		if gvr.Version == "" {
			return fmt.Errorf("config version is required, index=%d", index)
		}

		if gvr.Resource == "" {
			return fmt.Errorf("config resource is required, index=%d", index)
		}

		if _, existed := configGVRMap[gvr]; existed {
			return fmt.Errorf("config type %q is duplicated", gvr.String())
		}
		configGVRMap[gvr] = true
	}

	return nil
}
