// Package dependency implements host and service dependency checking.
package dependency

import (
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

const (
	DependenciesOK     = 0
	DependenciesFailed = 1
)

// CheckServiceDependencies checks notification or execution dependencies for a service.
// depType is objects.NotificationDependency or objects.ExecutionDependency.
func CheckServiceDependencies(svc *objects.Service, depType int, softStateDeps bool) int {
	var deps []*objects.ServiceDependency
	if depType == objects.NotificationDependency {
		deps = svc.NotifyDeps
	} else {
		deps = svc.ExecDeps
	}
	return checkServiceDeps(deps, depType, softStateDeps, nil)
}

func checkServiceDeps(deps []*objects.ServiceDependency, depType int, softStateDeps bool, visited map[*objects.Service]bool) int {
	if visited == nil {
		visited = make(map[*objects.Service]bool)
	}
	for _, dep := range deps {
		master := dep.Service
		if master == nil {
			continue
		}
		// Prevent infinite recursion
		if visited[master] {
			continue
		}

		// Choose correct failure options based on dep type
		var failOpts uint32
		if depType == objects.NotificationDependency {
			failOpts = dep.NotificationFailureOptions
		} else {
			failOpts = dep.ExecutionFailureOptions
		}
		if failOpts == 0 {
			continue
		}

		// Check dependency period
		if dep.DependencyPeriod != nil && !objects.InTimeperiod(dep.DependencyPeriod, time.Now()) {
			continue
		}

		// Determine state to check
		state := master.CurrentState
		if master.StateType == objects.StateTypeSoft && !softStateDeps {
			state = master.LastHardState
		}

		// Check if master's state matches any failure options
		if stateMatchesSvcFailOpts(state, failOpts) {
			return DependenciesFailed
		}

		// Check inherited parent dependencies
		if dep.InheritsParent {
			visited[master] = true
			var parentDeps []*objects.ServiceDependency
			if depType == objects.NotificationDependency {
				parentDeps = master.NotifyDeps
			} else {
				parentDeps = master.ExecDeps
			}
			if checkServiceDeps(parentDeps, depType, softStateDeps, visited) == DependenciesFailed {
				return DependenciesFailed
			}
		}
	}
	return DependenciesOK
}

// CheckHostDependencies checks notification or execution dependencies for a host.
func CheckHostDependencies(hst *objects.Host, depType int, softStateDeps bool) int {
	var deps []*objects.HostDependency
	if depType == objects.NotificationDependency {
		deps = hst.NotifyDeps
	} else {
		deps = hst.ExecDeps
	}
	return checkHostDeps(deps, depType, softStateDeps, nil)
}

func checkHostDeps(deps []*objects.HostDependency, depType int, softStateDeps bool, visited map[*objects.Host]bool) int {
	if visited == nil {
		visited = make(map[*objects.Host]bool)
	}
	for _, dep := range deps {
		master := dep.Host
		if master == nil {
			continue
		}
		if visited[master] {
			continue
		}

		var failOpts uint32
		if depType == objects.NotificationDependency {
			failOpts = dep.NotificationFailureOptions
		} else {
			failOpts = dep.ExecutionFailureOptions
		}
		if failOpts == 0 {
			continue
		}

		if dep.DependencyPeriod != nil && !objects.InTimeperiod(dep.DependencyPeriod, time.Now()) {
			continue
		}

		state := master.CurrentState
		if master.StateType == objects.StateTypeSoft && !softStateDeps {
			state = master.LastHardState
		}

		if stateMatchesHostFailOpts(state, failOpts) {
			return DependenciesFailed
		}

		if dep.InheritsParent {
			visited[master] = true
			var parentDeps []*objects.HostDependency
			if depType == objects.NotificationDependency {
				parentDeps = master.NotifyDeps
			} else {
				parentDeps = master.ExecDeps
			}
			if checkHostDeps(parentDeps, depType, softStateDeps, visited) == DependenciesFailed {
				return DependenciesFailed
			}
		}
	}
	return DependenciesOK
}

func stateMatchesSvcFailOpts(state int, opts uint32) bool {
	switch state {
	case objects.ServiceOK:
		return opts&objects.OptOK != 0
	case objects.ServiceWarning:
		return opts&objects.OptWarning != 0
	case objects.ServiceCritical:
		return opts&objects.OptCritical != 0
	case objects.ServiceUnknown:
		return opts&objects.OptUnknown != 0
	}
	return false
}

func stateMatchesHostFailOpts(state int, opts uint32) bool {
	switch state {
	case objects.HostUp:
		return opts&objects.OptOK != 0
	case objects.HostDown:
		return opts&objects.OptDown != 0
	case objects.HostUnreachable:
		return opts&objects.OptUnreachable != 0
	}
	return false
}
