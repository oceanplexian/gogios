package nrdp

import "github.com/oceanplexian/gogios/internal/objects"

type dynamicServiceDependencyRule struct {
	dependent string
	master    string
}

var dynamicServiceDependencyRules = []dynamicServiceDependencyRule{
	{
		dependent: "Systemd FD Exhaustion",
		master:    "K8s Node Ready",
	},
}

const dynamicServiceDependencyCriteria = "w,u,c,p"

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
		master := d.store.GetService(hostname, rule.master)
		dependent := d.store.GetService(hostname, rule.dependent)
		if master == nil || dependent == nil {
			continue
		}
		if d.serviceDependencyExists(master, dependent) {
			continue
		}
		dep := &objects.ServiceDependency{
			Host:                       host,
			Service:                    master,
			DependentHost:              host,
			DependentService:           dependent,
			DependencyPeriod:           d.store.GetTimeperiod("24x7"),
			ExecutionFailureOptions:    dynamicServiceDependencyFailureOptions,
			NotificationFailureOptions: dynamicServiceDependencyFailureOptions,
		}
		d.store.AddServiceDependency(dep)
	}
}

func (d *DynamicTracker) serviceDependencyExists(master, dependent *objects.Service) bool {
	for _, dep := range d.store.ServiceDependencies {
		if dep.Service == master && dep.DependentService == dependent {
			return true
		}
	}
	return false
}
