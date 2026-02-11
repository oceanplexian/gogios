package config

import (
	"fmt"

	"github.com/oceanplexian/gogios/internal/objects"
)

// Validate runs pre-flight checks similar to Nagios's pre_flight_check().
func Validate(store *objects.ObjectStore) []error {
	var errs []error

	// Validate hosts
	for _, h := range store.Hosts {
		if h.Name == "" {
			errs = append(errs, fmt.Errorf("host has no host_name"))
		}
		if h.Alias == "" {
			errs = append(errs, fmt.Errorf("host '%s': missing alias", h.Name))
		}
		if h.MaxCheckAttempts < 1 {
			errs = append(errs, fmt.Errorf("host '%s': max_check_attempts must be >= 1 (got %d)", h.Name, h.MaxCheckAttempts))
		}
		if len(h.ContactGroups) == 0 && len(h.Contacts) == 0 {
			errs = append(errs, fmt.Errorf("host '%s': has no contacts or contact_groups", h.Name))
		}
	}

	// Validate services
	for _, svc := range store.Services {
		if svc.Host == nil {
			errs = append(errs, fmt.Errorf("service '%s': has no host", svc.Description))
			continue
		}
		if svc.Description == "" {
			errs = append(errs, fmt.Errorf("service on host '%s': missing service_description", svc.Host.Name))
		}
		if svc.MaxCheckAttempts < 1 {
			errs = append(errs, fmt.Errorf("service '%s/%s': max_check_attempts must be >= 1 (got %d)",
				svc.Host.Name, svc.Description, svc.MaxCheckAttempts))
		}
		if svc.CheckCommand == nil {
			errs = append(errs, fmt.Errorf("service '%s/%s': missing check_command",
				svc.Host.Name, svc.Description))
		}
		if len(svc.ContactGroups) == 0 && len(svc.Contacts) == 0 {
			errs = append(errs, fmt.Errorf("service '%s/%s': has no contacts or contact_groups",
				svc.Host.Name, svc.Description))
		}
	}

	// Validate contacts
	for _, c := range store.Contacts {
		if c.Name == "" {
			errs = append(errs, fmt.Errorf("contact has no contact_name"))
		}
	}

	// Validate contact groups
	for _, cg := range store.ContactGroups {
		if cg.Name == "" {
			errs = append(errs, fmt.Errorf("contactgroup has no contactgroup_name"))
		}
	}

	// Check for circular host parent dependencies
	if err := checkCircularHostParents(store); err != nil {
		errs = append(errs, err)
	}

	// Check for circular host dependencies
	if err := checkCircularHostDeps(store); err != nil {
		errs = append(errs, err)
	}

	// Check for circular service dependencies
	if err := checkCircularServiceDeps(store); err != nil {
		errs = append(errs, err)
	}

	// Check for circular timeperiod exclusions
	if err := checkCircularTimeperiodExclusions(store); err != nil {
		errs = append(errs, err)
	}

	return errs
}

func checkCircularHostParents(store *objects.ObjectStore) error {
	for _, h := range store.Hosts {
		visited := make(map[string]bool)
		if err := walkHostParents(h, visited); err != nil {
			return err
		}
	}
	return nil
}

func walkHostParents(h *objects.Host, visited map[string]bool) error {
	if visited[h.Name] {
		return fmt.Errorf("circular parent dependency detected for host '%s'", h.Name)
	}
	visited[h.Name] = true
	for _, p := range h.Parents {
		if err := walkHostParents(p, visited); err != nil {
			return err
		}
	}
	delete(visited, h.Name)
	return nil
}

func checkCircularHostDeps(store *objects.ObjectStore) error {
	// Build adjacency list
	adj := make(map[string][]string)
	for _, hd := range store.HostDependencies {
		if hd.DependentHost != nil && hd.Host != nil {
			adj[hd.DependentHost.Name] = append(adj[hd.DependentHost.Name], hd.Host.Name)
		}
	}
	for _, h := range store.Hosts {
		visited := make(map[string]bool)
		if err := walkDeps(h.Name, adj, visited); err != nil {
			return fmt.Errorf("circular host dependency: %w", err)
		}
	}
	return nil
}

func checkCircularServiceDeps(store *objects.ObjectStore) error {
	adj := make(map[svcKey][]svcKey)
	for _, sd := range store.ServiceDependencies {
		if sd.DependentService != nil && sd.Service != nil {
			dk := svcKey{sd.DependentHost.Name, sd.DependentService.Description}
			mk := svcKey{sd.Host.Name, sd.Service.Description}
			adj[dk] = append(adj[dk], mk)
		}
	}
	for k := range adj {
		visited := make(map[svcKey]bool)
		if err := walkServiceDeps(k, adj, visited); err != nil {
			return err
		}
	}
	return nil
}

type svcKey struct{ host, desc string }

func walkServiceDeps(key svcKey, adj map[svcKey][]svcKey, visited map[svcKey]bool) error {
	if visited[key] {
		return fmt.Errorf("circular service dependency detected for '%s/%s'", key.host, key.desc)
	}
	visited[key] = true
	for _, dep := range adj[key] {
		if err := walkServiceDeps(dep, adj, visited); err != nil {
			return err
		}
	}
	delete(visited, key)
	return nil
}

func walkDeps(name string, adj map[string][]string, visited map[string]bool) error {
	if visited[name] {
		return fmt.Errorf("circular dependency at '%s'", name)
	}
	visited[name] = true
	for _, dep := range adj[name] {
		if err := walkDeps(dep, adj, visited); err != nil {
			return err
		}
	}
	delete(visited, name)
	return nil
}

func checkCircularTimeperiodExclusions(store *objects.ObjectStore) error {
	for _, tp := range store.Timeperiods {
		visited := make(map[string]bool)
		if err := walkTPExclusions(tp, visited); err != nil {
			return err
		}
	}
	return nil
}

func walkTPExclusions(tp *objects.Timeperiod, visited map[string]bool) error {
	if visited[tp.Name] {
		return fmt.Errorf("circular timeperiod exclusion detected for '%s'", tp.Name)
	}
	visited[tp.Name] = true
	for _, exc := range tp.Exclusions {
		if err := walkTPExclusions(exc, visited); err != nil {
			return err
		}
	}
	delete(visited, tp.Name)
	return nil
}
