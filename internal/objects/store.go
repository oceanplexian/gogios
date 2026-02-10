package objects

import (
	"fmt"
	"sync"
)

type ObjectStore struct {
	// Mu protects mutable runtime state on Host/Service objects.
	// The scheduler takes a write lock when processing check results;
	// livestatus readers take a read lock when executing queries.
	Mu sync.RWMutex
	Hosts              []*Host
	Services           []*Service
	Commands           []*Command
	Contacts           []*Contact
	ContactGroups      []*ContactGroup
	Timeperiods        []*Timeperiod
	HostGroups         []*HostGroup
	ServiceGroups      []*ServiceGroup
	HostDependencies   []*HostDependency
	ServiceDependencies []*ServiceDependency
	HostEscalations    []*HostEscalation
	ServiceEscalations []*ServiceEscalation

	hostsByName         map[string]*Host
	servicesByHostDesc  map[string]*Service // "hostname\tsvc_description"
	commandsByName      map[string]*Command
	contactsByName      map[string]*Contact
	contactGroupsByName map[string]*ContactGroup
	timeperiodsByName   map[string]*Timeperiod
	hostGroupsByName    map[string]*HostGroup
	serviceGroupsByName map[string]*ServiceGroup
}

func NewObjectStore() *ObjectStore {
	return &ObjectStore{
		hostsByName:         make(map[string]*Host),
		servicesByHostDesc:  make(map[string]*Service),
		commandsByName:      make(map[string]*Command),
		contactsByName:      make(map[string]*Contact),
		contactGroupsByName: make(map[string]*ContactGroup),
		timeperiodsByName:   make(map[string]*Timeperiod),
		hostGroupsByName:    make(map[string]*HostGroup),
		serviceGroupsByName: make(map[string]*ServiceGroup),
	}
}

func svcKey(hostName, desc string) string {
	return hostName + "\t" + desc
}

func (s *ObjectStore) AddHost(h *Host) error {
	if _, exists := s.hostsByName[h.Name]; exists {
		return fmt.Errorf("duplicate host: %s", h.Name)
	}
	s.Hosts = append(s.Hosts, h)
	s.hostsByName[h.Name] = h
	return nil
}

func (s *ObjectStore) GetHost(name string) *Host {
	return s.hostsByName[name]
}

func (s *ObjectStore) AddService(svc *Service) error {
	key := svcKey(svc.Host.Name, svc.Description)
	if _, exists := s.servicesByHostDesc[key]; exists {
		return fmt.Errorf("duplicate service: %s/%s", svc.Host.Name, svc.Description)
	}
	s.Services = append(s.Services, svc)
	s.servicesByHostDesc[key] = svc
	return nil
}

func (s *ObjectStore) GetService(hostName, desc string) *Service {
	return s.servicesByHostDesc[svcKey(hostName, desc)]
}

func (s *ObjectStore) AddCommand(c *Command) error {
	if _, exists := s.commandsByName[c.Name]; exists {
		return fmt.Errorf("duplicate command: %s", c.Name)
	}
	s.Commands = append(s.Commands, c)
	s.commandsByName[c.Name] = c
	return nil
}

func (s *ObjectStore) GetCommand(name string) *Command {
	return s.commandsByName[name]
}

func (s *ObjectStore) AddContact(c *Contact) error {
	if _, exists := s.contactsByName[c.Name]; exists {
		return fmt.Errorf("duplicate contact: %s", c.Name)
	}
	s.Contacts = append(s.Contacts, c)
	s.contactsByName[c.Name] = c
	return nil
}

func (s *ObjectStore) GetContact(name string) *Contact {
	return s.contactsByName[name]
}

func (s *ObjectStore) AddContactGroup(cg *ContactGroup) error {
	if _, exists := s.contactGroupsByName[cg.Name]; exists {
		return fmt.Errorf("duplicate contactgroup: %s", cg.Name)
	}
	s.ContactGroups = append(s.ContactGroups, cg)
	s.contactGroupsByName[cg.Name] = cg
	return nil
}

func (s *ObjectStore) GetContactGroup(name string) *ContactGroup {
	return s.contactGroupsByName[name]
}

func (s *ObjectStore) AddTimeperiod(tp *Timeperiod) error {
	if _, exists := s.timeperiodsByName[tp.Name]; exists {
		return fmt.Errorf("duplicate timeperiod: %s", tp.Name)
	}
	s.Timeperiods = append(s.Timeperiods, tp)
	s.timeperiodsByName[tp.Name] = tp
	return nil
}

func (s *ObjectStore) GetTimeperiod(name string) *Timeperiod {
	return s.timeperiodsByName[name]
}

func (s *ObjectStore) AddHostGroup(hg *HostGroup) error {
	if _, exists := s.hostGroupsByName[hg.Name]; exists {
		return fmt.Errorf("duplicate hostgroup: %s", hg.Name)
	}
	s.HostGroups = append(s.HostGroups, hg)
	s.hostGroupsByName[hg.Name] = hg
	return nil
}

func (s *ObjectStore) GetHostGroup(name string) *HostGroup {
	return s.hostGroupsByName[name]
}

func (s *ObjectStore) AddServiceGroup(sg *ServiceGroup) error {
	if _, exists := s.serviceGroupsByName[sg.Name]; exists {
		return fmt.Errorf("duplicate servicegroup: %s", sg.Name)
	}
	s.ServiceGroups = append(s.ServiceGroups, sg)
	s.serviceGroupsByName[sg.Name] = sg
	return nil
}

func (s *ObjectStore) GetServiceGroup(name string) *ServiceGroup {
	return s.serviceGroupsByName[name]
}

func (s *ObjectStore) AddHostDependency(hd *HostDependency) {
	s.HostDependencies = append(s.HostDependencies, hd)
	// Wire up to the dependent host's dependency lists
	if hd.DependentHost != nil {
		if hd.NotificationFailureOptions != 0 {
			hd.DependentHost.NotifyDeps = append(hd.DependentHost.NotifyDeps, hd)
		}
		if hd.ExecutionFailureOptions != 0 {
			hd.DependentHost.ExecDeps = append(hd.DependentHost.ExecDeps, hd)
		}
	}
}

func (s *ObjectStore) AddServiceDependency(sd *ServiceDependency) {
	s.ServiceDependencies = append(s.ServiceDependencies, sd)
	// Wire up to the dependent service's dependency lists
	if sd.DependentService != nil {
		if sd.NotificationFailureOptions != 0 {
			sd.DependentService.NotifyDeps = append(sd.DependentService.NotifyDeps, sd)
		}
		if sd.ExecutionFailureOptions != 0 {
			sd.DependentService.ExecDeps = append(sd.DependentService.ExecDeps, sd)
		}
	}
}

func (s *ObjectStore) AddHostEscalation(he *HostEscalation) {
	s.HostEscalations = append(s.HostEscalations, he)
	// Wire up to the host's escalation list
	if he.Host != nil {
		he.Host.Escalations = append(he.Host.Escalations, he)
	}
}

func (s *ObjectStore) AddServiceEscalation(se *ServiceEscalation) {
	s.ServiceEscalations = append(s.ServiceEscalations, se)
	// Wire up to the service's escalation list
	if se.Service != nil {
		se.Service.Escalations = append(se.Service.Escalations, se)
	}
}

// GetServicesForHost returns all services associated with a host.
func (s *ObjectStore) GetServicesForHost(hostName string) []*Service {
	var result []*Service
	for _, svc := range s.Services {
		if svc.Host != nil && svc.Host.Name == hostName {
			result = append(result, svc)
		}
	}
	return result
}

// RemoveHost removes a host and all its services from the store.
// Caller must hold the write lock.
func (s *ObjectStore) RemoveHost(name string) {
	host := s.hostsByName[name]
	if host == nil {
		return
	}
	// Remove all services for this host first
	var keptServices []*Service
	for _, svc := range s.Services {
		if svc.Host != nil && svc.Host.Name == name {
			delete(s.servicesByHostDesc, svcKey(name, svc.Description))
		} else {
			keptServices = append(keptServices, svc)
		}
	}
	s.Services = keptServices

	// Remove the host
	delete(s.hostsByName, name)
	for i, h := range s.Hosts {
		if h.Name == name {
			s.Hosts = append(s.Hosts[:i], s.Hosts[i+1:]...)
			break
		}
	}
}

// RemoveService removes a single service from the store.
// Caller must hold the write lock.
func (s *ObjectStore) RemoveService(hostName, desc string) {
	key := svcKey(hostName, desc)
	if _, exists := s.servicesByHostDesc[key]; !exists {
		return
	}
	delete(s.servicesByHostDesc, key)
	for i, svc := range s.Services {
		if svc.Host != nil && svc.Host.Name == hostName && svc.Description == desc {
			s.Services = append(s.Services[:i], s.Services[i+1:]...)
			break
		}
	}
}
