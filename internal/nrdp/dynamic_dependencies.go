package nrdp

import (
	"path"
	"strings"

	"github.com/oceanplexian/gogios/internal/objects"
)

type dynamicServiceDependencyRule struct {
	hostPattern        string
	master             string
	dependentPattern   string
	excludedDependents map[string]bool
}

var dynamicServiceDependencyRules = []dynamicServiceDependencyRule{
	{
		hostPattern:      "*",
		master:           nrdcServiceName,
		dependentPattern: "*",
		excludedDependents: map[string]bool{
			nrdcServiceName: true,
		},
	},
	{
		hostPattern:      "k8s-local-*.fieldio.com",
		master:           k8sNodeReadyServiceName,
		dependentPattern: "*",
		excludedDependents: map[string]bool{
			nrdcServiceName:         true,
			k8sNodeReadyServiceName: true,
		},
	},
}

const (
	dynamicServiceExecutionDependencyCriteria    = "n"
	dynamicServiceNotificationDependencyCriteria = "w,u,c,p"
)

const dynamicServiceDependencyFailureOptions = objects.OptWarning |
	objects.OptUnknown |
	objects.OptCritical |
	objects.OptPending

// ensureDynamicServiceDependenciesForHost wires known per-host passive service
// dependencies when both sides are present. Caller must hold store.Mu.
func (d *DynamicTracker) ensureDynamicServiceDependenciesForHost(hostname string) {
	host := d.store.GetHost(hostname)
	if host == nil {
		return
	}
	for _, rule := range dynamicServiceDependencyRules {
		if !rule.matchesHost(hostname) {
			continue
		}
		master := d.store.GetService(hostname, rule.master)
		if master == nil {
			continue
		}
		for _, dependent := range d.store.GetServicesForHost(hostname) {
			if dependent == nil ||
				dependent.ActiveChecksEnabled ||
				!dependent.PassiveChecksEnabled ||
				!rule.matchesDependent(dependent.Description) {
				continue
			}
			if d.serviceDependencyExists(master, dependent) {
				continue
			}
			dep := &objects.ServiceDependency{
				Host:             host,
				Service:          master,
				DependentHost:    host,
				DependentService: dependent,
				DependencyPeriod: d.store.GetTimeperiod("24x7"),
				// Keep freshness checks executable so stale children become
				// explicit UNKNOWN instead of retaining an old green result.
				// The hierarchy suppresses their notifications, not state.
				ExecutionFailureOptions:    0,
				NotificationFailureOptions: dynamicServiceDependencyFailureOptions,
			}
			d.store.AddServiceDependency(dep)
		}
	}
}

func (r dynamicServiceDependencyRule) matchesHost(hostname string) bool {
	matched, err := path.Match(r.hostPattern, hostname)
	return err == nil && matched
}

func (r dynamicServiceDependencyRule) matchesDependent(description string) bool {
	if description == r.master || r.excludedDependents[description] {
		return false
	}
	// path.Match treats "/" as a path separator, so "*" does not match
	// service descriptions such as "test.fn2.ai HTTPS /health". Nagios
	// service descriptions are opaque names, not filesystem paths. Replace
	// slashes in both operands before applying the glob so every character in
	// a service name participates in the same wildcard namespace.
	const slashPlaceholder = "\uE000"
	pattern := strings.ReplaceAll(r.dependentPattern, "/", slashPlaceholder)
	name := strings.ReplaceAll(description, "/", slashPlaceholder)
	matched, err := path.Match(pattern, name)
	return err == nil && matched
}

func dynamicHostGroupNames(hostname string) []string {
	names := []string{"discovered-hosts"}
	switch {
	case strings.HasPrefix(hostname, "k8s-local-"):
		names = append(names, "k8s-local-nodes")
	case strings.HasPrefix(hostname, "nas-"), strings.HasPrefix(hostname, "ssd-"):
		names = append(names, "storage-servers")
	case strings.HasPrefix(hostname, "ai-"):
		names = append(names, "ai-servers")
	}
	return names
}

func dynamicServiceGroupNames(hostname, description string) []string {
	names := []string{"passive-services"}
	if strings.HasPrefix(hostname, "k8s-local-") {
		names = append(names, "kubernetes-node-services")
	}
	if hostname == "central" && (strings.Contains(description, "K8s") ||
		strings.Contains(description, "Containerd") ||
		strings.Contains(description, "Pod ") ||
		strings.Contains(description, "Shell Launch")) {
		names = append(names, "kubernetes-cluster-services")
	}
	if strings.Contains(strings.ToLower(description), "dns") ||
		strings.Contains(strings.ToLower(description), "coredns") {
		names = append(names, "dns-services")
	}
	if strings.HasPrefix(hostname, "fn2") ||
		strings.HasPrefix(strings.ToLower(description), "fn2 ") {
		names = append(names, "fn2-services")
	}
	return names
}

func (d *DynamicTracker) serviceDependencyExists(master, dependent *objects.Service) bool {
	for _, dep := range d.store.ServiceDependencies {
		if dep.Service == master && dep.DependentService == dependent {
			return true
		}
	}
	return false
}
