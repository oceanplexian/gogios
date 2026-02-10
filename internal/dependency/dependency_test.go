package dependency

import (
	"testing"

	"github.com/oceanplexian/gogios/internal/objects"
)

func TestCheckServiceDependencies_NoDeps(t *testing.T) {
	svc := &objects.Service{}
	if CheckServiceDependencies(svc, objects.NotificationDependency, false) != DependenciesOK {
		t.Error("expected OK with no dependencies")
	}
}

func TestCheckServiceDependencies_MasterCritical(t *testing.T) {
	master := &objects.Service{
		CurrentState: objects.ServiceCritical,
		StateType:    objects.StateTypeHard,
	}
	dep := &objects.ServiceDependency{
		Service:                    master,
		NotificationFailureOptions: objects.OptCritical,
	}
	svc := &objects.Service{
		NotifyDeps: []*objects.ServiceDependency{dep},
	}
	if CheckServiceDependencies(svc, objects.NotificationDependency, false) != DependenciesFailed {
		t.Error("expected FAILED when master is critical")
	}
}

func TestCheckServiceDependencies_SoftStateIgnored(t *testing.T) {
	master := &objects.Service{
		CurrentState:  objects.ServiceCritical,
		LastHardState:  objects.ServiceOK,
		StateType:     objects.StateTypeSoft,
	}
	dep := &objects.ServiceDependency{
		Service:                    master,
		NotificationFailureOptions: objects.OptCritical,
	}
	svc := &objects.Service{
		NotifyDeps: []*objects.ServiceDependency{dep},
	}
	// With softStateDeps=false, should use last hard state (OK)
	if CheckServiceDependencies(svc, objects.NotificationDependency, false) != DependenciesOK {
		t.Error("expected OK when soft state is ignored and hard state is OK")
	}
	// With softStateDeps=true, should use current state (CRITICAL)
	if CheckServiceDependencies(svc, objects.NotificationDependency, true) != DependenciesFailed {
		t.Error("expected FAILED when soft state deps enabled")
	}
}

func TestCheckServiceDependencies_InheritsParent(t *testing.T) {
	grandmaster := &objects.Service{
		CurrentState: objects.ServiceWarning,
		StateType:    objects.StateTypeHard,
	}
	grandDep := &objects.ServiceDependency{
		Service:                    grandmaster,
		NotificationFailureOptions: objects.OptWarning,
	}
	master := &objects.Service{
		CurrentState: objects.ServiceOK,
		StateType:    objects.StateTypeHard,
		NotifyDeps:   []*objects.ServiceDependency{grandDep},
	}
	dep := &objects.ServiceDependency{
		Service:                    master,
		NotificationFailureOptions: objects.OptCritical,
		InheritsParent:             true,
	}
	svc := &objects.Service{
		NotifyDeps: []*objects.ServiceDependency{dep},
	}
	if CheckServiceDependencies(svc, objects.NotificationDependency, false) != DependenciesFailed {
		t.Error("expected FAILED through inherited parent")
	}
}

func TestCheckHostDependencies_MasterDown(t *testing.T) {
	master := &objects.Host{
		CurrentState: objects.HostDown,
		StateType:    objects.StateTypeHard,
	}
	dep := &objects.HostDependency{
		Host:                       master,
		NotificationFailureOptions: objects.OptDown,
	}
	hst := &objects.Host{
		NotifyDeps: []*objects.HostDependency{dep},
	}
	if CheckHostDependencies(hst, objects.NotificationDependency, false) != DependenciesFailed {
		t.Error("expected FAILED when master host is down")
	}
}

func TestCheckHostDependencies_ExecType(t *testing.T) {
	master := &objects.Host{
		CurrentState: objects.HostDown,
		StateType:    objects.StateTypeHard,
	}
	dep := &objects.HostDependency{
		Host:                      master,
		ExecutionFailureOptions:   objects.OptDown,
	}
	hst := &objects.Host{
		ExecDeps: []*objects.HostDependency{dep},
	}
	if CheckHostDependencies(hst, objects.ExecutionDependency, false) != DependenciesFailed {
		t.Error("expected FAILED for exec dependency")
	}
}
