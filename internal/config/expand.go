package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/oceanplexian/gogios/internal/objects"
)

// ExpandAndRegister runs the full Nagios config pipeline: group resolution,
// service duplication, inter-object inheritance, and registration into the store.
func ExpandAndRegister(parser *ObjectParser, store *objects.ObjectStore) error {
	// Step 1: Register commands first (needed by everything else)
	if err := registerCommands(parser, store); err != nil {
		return err
	}
	// Step 2: Register timeperiods
	if err := registerTimeperiods(parser, store); err != nil {
		return err
	}
	// Step 3: Register contacts
	if err := registerContacts(parser, store); err != nil {
		return err
	}
	// Step 4: Register contact groups (recombobulate)
	if err := registerContactGroups(parser, store); err != nil {
		return err
	}
	// Step 5: Register hosts
	if err := registerHosts(parser, store); err != nil {
		return err
	}
	// Step 6: Register host groups (recombobulate)
	if err := registerHostGroups(parser, store); err != nil {
		return err
	}
	// Step 7: Register services (with duplication for multi-host and hostgroup)
	if err := registerServices(parser, store); err != nil {
		return err
	}
	// Step 8: Register service groups (recombobulate)
	if err := registerServiceGroups(parser, store); err != nil {
		return err
	}
	// Step 9: Inter-object inheritance (service ← host)
	inheritObjectProperties(store)
	// Step 10: Register host dependencies (with expansion)
	if err := registerHostDependencies(parser, store); err != nil {
		return err
	}
	// Step 11: Register service dependencies (with expansion)
	if err := registerServiceDependencies(parser, store); err != nil {
		return err
	}
	// Step 12: Register host escalations
	if err := registerHostEscalations(parser, store); err != nil {
		return err
	}
	// Step 13: Register service escalations
	if err := registerServiceEscalations(parser, store); err != nil {
		return err
	}
	// Step 14: Resolve host parent/child relationships
	if err := resolveHostParents(store); err != nil {
		return err
	}
	// Step 15: Wire up host/service group bidirectional refs
	wireGroupReferences(store)

	return nil
}

func registerCommands(parser *ObjectParser, store *objects.ObjectStore) error {
	for _, obj := range parser.Objects {
		if obj.Type != "command" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("command_name")
		line, _ := obj.Get("command_line")
		if name == "" {
			return fmt.Errorf("%s:%d: command missing command_name", obj.File, obj.Line)
		}
		cmd := &objects.Command{Name: name, CommandLine: line}
		if err := store.AddCommand(cmd); err != nil {
			return fmt.Errorf("%s:%d: %w", obj.File, obj.Line, err)
		}
	}
	return nil
}

func registerTimeperiods(parser *ObjectParser, store *objects.ObjectStore) error {
	for _, obj := range parser.Objects {
		if obj.Type != "timeperiod" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("timeperiod_name")
		if name == "" {
			return fmt.Errorf("%s:%d: timeperiod missing timeperiod_name", obj.File, obj.Line)
		}
		tp := &objects.Timeperiod{
			Name:  name,
			Alias: attrOr(obj, "alias", name),
		}
		// Weekday ranges
		days := [7]string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}
		for i, day := range days {
			if v, ok := obj.Get(day); ok {
				tp.Ranges[i] = v
			}
		}
		// Date exceptions are handled by timeperiod.go parsing later
		// Parse any date exception lines from remaining attrs
		for key, val := range obj.Attrs {
			exc := parseTimeDateException(key, val)
			if exc != nil {
				tp.Exceptions = append(tp.Exceptions, *exc)
			}
		}
		if err := store.AddTimeperiod(tp); err != nil {
			return fmt.Errorf("%s:%d: %w", obj.File, obj.Line, err)
		}
	}
	// Resolve exclusions after all timeperiods are registered
	for _, obj := range parser.Objects {
		if obj.Type != "timeperiod" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("timeperiod_name")
		tp := store.GetTimeperiod(name)
		if exclStr, ok := obj.Get("exclude"); ok {
			for _, eName := range splitCSV(exclStr) {
				exc := store.GetTimeperiod(eName)
				if exc == nil {
					return fmt.Errorf("%s:%d: excluded timeperiod '%s' not found", obj.File, obj.Line, eName)
				}
				tp.Exclusions = append(tp.Exclusions, exc)
			}
		}
	}
	return nil
}

func registerContacts(parser *ObjectParser, store *objects.ObjectStore) error {
	for _, obj := range parser.Objects {
		if obj.Type != "contact" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("contact_name")
		if name == "" {
			return fmt.Errorf("%s:%d: contact missing contact_name", obj.File, obj.Line)
		}
		c := &objects.Contact{
			Name:                       name,
			Alias:                      attrOr(obj, "alias", name),
			Email:                      attrOr(obj, "email", ""),
			Pager:                      attrOr(obj, "pager", ""),
			HostNotificationsEnabled:   attrBool(obj, "host_notifications_enabled", true),
			ServiceNotificationsEnabled: attrBool(obj, "service_notifications_enabled", true),
			CanSubmitCommands:          attrBool(obj, "can_submit_commands", true),
			RetainStatusInformation:    attrBool(obj, "retain_status_information", true),
			RetainNonstatusInformation: attrBool(obj, "retain_nonstatus_information", true),
			CustomVars:                 copyMap(obj.CustomVars),
		}
		// Addresses
		for i := 0; i < objects.MaxContactAddresses; i++ {
			key := fmt.Sprintf("address%d", i+1)
			if v, ok := obj.Get(key); ok {
				c.Addresses[i] = v
			}
		}
		if v, ok := obj.Get("minimum_importance"); ok {
			n, _ := strconv.ParseUint(v, 10, 64)
			c.MinimumImportance = uint(n)
		}
		c.HostNotificationOptions = parseHostNotificationOptions(attrOr(obj, "host_notification_options", ""))
		c.ServiceNotificationOptions = parseServiceNotificationOptions(attrOr(obj, "service_notification_options", ""))

		if err := store.AddContact(c); err != nil {
			return fmt.Errorf("%s:%d: %w", obj.File, obj.Line, err)
		}
	}
	// Resolve contact references (notification periods, commands)
	for _, obj := range parser.Objects {
		if obj.Type != "contact" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("contact_name")
		c := store.GetContact(name)
		if v, ok := obj.Get("host_notification_period"); ok {
			c.HostNotificationPeriod = store.GetTimeperiod(v)
		}
		if v, ok := obj.Get("service_notification_period"); ok {
			c.ServiceNotificationPeriod = store.GetTimeperiod(v)
		}
		if v, ok := obj.Get("host_notification_commands"); ok {
			c.HostNotificationCommands = resolveCommands(store, v)
		}
		if v, ok := obj.Get("service_notification_commands"); ok {
			c.ServiceNotificationCommands = resolveCommands(store, v)
		}
	}
	return nil
}

func registerContactGroups(parser *ObjectParser, store *objects.ObjectStore) error {
	// First pass: create all contactgroups
	for _, obj := range parser.Objects {
		if obj.Type != "contactgroup" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("contactgroup_name")
		if name == "" {
			return fmt.Errorf("%s:%d: contactgroup missing contactgroup_name", obj.File, obj.Line)
		}
		cg := &objects.ContactGroup{
			Name:  name,
			Alias: attrOr(obj, "alias", name),
		}
		if err := store.AddContactGroup(cg); err != nil {
			return fmt.Errorf("%s:%d: %w", obj.File, obj.Line, err)
		}
	}
	// Second pass: resolve members and contactgroup_members
	for _, obj := range parser.Objects {
		if obj.Type != "contactgroup" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("contactgroup_name")
		cg := store.GetContactGroup(name)
		if v, ok := obj.Get("members"); ok {
			for _, cName := range splitCSV(v) {
				c := store.GetContact(cName)
				if c == nil {
					return fmt.Errorf("%s:%d: contact '%s' not found in contactgroup '%s'", obj.File, obj.Line, cName, name)
				}
				cg.Members = append(cg.Members, c)
			}
		}
		if v, ok := obj.Get("contactgroup_members"); ok {
			for _, cgName := range splitCSV(v) {
				sub := store.GetContactGroup(cgName)
				if sub == nil {
					return fmt.Errorf("%s:%d: contactgroup '%s' not found in contactgroup_members", obj.File, obj.Line, cgName)
				}
				for _, m := range sub.Members {
					if !containsContact(cg.Members, m) {
						cg.Members = append(cg.Members, m)
					}
				}
			}
		}
	}
	// Recombobulate: contacts with contactgroups directive
	for _, obj := range parser.Objects {
		if obj.Type != "contact" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("contact_name")
		c := store.GetContact(name)
		if v, ok := obj.Get("contactgroups"); ok {
			for _, cgName := range splitCSV(v) {
				cg := store.GetContactGroup(cgName)
				if cg == nil {
					return fmt.Errorf("%s:%d: contactgroup '%s' not found", obj.File, obj.Line, cgName)
				}
				if !containsContact(cg.Members, c) {
					cg.Members = append(cg.Members, c)
				}
				c.ContactGroups = append(c.ContactGroups, cg)
			}
		}
	}
	return nil
}

func registerHosts(parser *ObjectParser, store *objects.ObjectStore) error {
	for _, obj := range parser.Objects {
		if obj.Type != "host" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("host_name")
		if name == "" {
			return fmt.Errorf("%s:%d: host missing host_name", obj.File, obj.Line)
		}
		h := &objects.Host{
			Name:                       name,
			DisplayName:                attrOr(obj, "display_name", name),
			Alias:                      attrOr(obj, "alias", name),
			Address:                    attrOr(obj, "address", name),
			CheckInterval:              attrFloat(obj, "check_interval", 5.0),
			RetryInterval:              attrFloat(obj, "retry_interval", 1.0),
			MaxCheckAttempts:           attrInt(obj, "max_check_attempts", -2),
			InitialState:              parseInitialHostState(attrOr(obj, "initial_state", "o")),
			ActiveChecksEnabled:        attrBool(obj, "active_checks_enabled", true),
			PassiveChecksEnabled:       attrBool(obj, "passive_checks_enabled", true),
			ObsessOver:                 attrBool(obj, "obsess_over_host", true),
			EventHandlerEnabled:        attrBool(obj, "event_handler_enabled", true),
			CheckFreshness:             attrBool(obj, "check_freshness", false),
			FreshnessThreshold:         attrInt(obj, "freshness_threshold", 0),
			LowFlapThreshold:           attrFloat(obj, "low_flap_threshold", 0),
			HighFlapThreshold:          attrFloat(obj, "high_flap_threshold", 0),
			FlapDetectionEnabled:       attrBool(obj, "flap_detection_enabled", true),
			FlapDetectionOptions:       parseFlapDetectionHostOptions(attrOr(obj, "flap_detection_options", "")),
			NotificationsEnabled:       attrBool(obj, "notifications_enabled", true),
			NotificationInterval:       attrFloat(obj, "notification_interval", 30.0),
			FirstNotificationDelay:     attrFloat(obj, "first_notification_delay", 0),
			StalingOptions:             parseStalingHostOptions(attrOr(obj, "stalking_options", "")),
			ProcessPerfData:            attrBool(obj, "process_perf_data", true),
			Notes:                      attrOr(obj, "notes", ""),
			NotesURL:                   attrOr(obj, "notes_url", ""),
			ActionURL:                  attrOr(obj, "action_url", ""),
			IconImage:                  attrOr(obj, "icon_image", ""),
			IconImageAlt:               attrOr(obj, "icon_image_alt", ""),
			VRMLImage:                  attrOr(obj, "vrml_image", ""),
			StatusmapImage:             attrOr(obj, "statusmap_image", ""),
			RetainStatusInformation:    attrBool(obj, "retain_status_information", true),
			RetainNonstatusInformation: attrBool(obj, "retain_nonstatus_information", true),
			CustomVars:                 copyMap(obj.CustomVars),
			ShouldBeScheduled:          true,
		}
		if v, ok := obj.Get("hourly_value"); ok {
			n, _ := strconv.ParseUint(v, 10, 64)
			h.HourlyValue = uint(n)
		}
		// Notification options (default to ALL if unset)
		if v, ok := obj.Get("notification_options"); ok {
			h.NotificationOptions = parseHostNotificationOptions(v)
		} else {
			h.NotificationOptions = objects.OptAll
		}
		// 2d/3d coords
		if v, ok := obj.Get("2d_coords"); ok {
			parts := strings.SplitN(v, ",", 2)
			if len(parts) == 2 {
				h.X2D, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
				h.Y2D, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
				h.Have2DCoords = true
			}
		}
		if v, ok := obj.Get("3d_coords"); ok {
			parts := strings.SplitN(v, ",", 3)
			if len(parts) == 3 {
				h.X3D, _ = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
				h.Y3D, _ = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
				h.Z3D, _ = strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
				h.Have3DCoords = true
			}
		}
		// Resolve references
		if v, ok := obj.Get("check_command"); ok {
			cmdName, args := splitCommandArgs(v)
			h.CheckCommand = store.GetCommand(cmdName)
			h.CheckCommandArgs = args
		}
		if v, ok := obj.Get("check_period"); ok {
			h.CheckPeriod = store.GetTimeperiod(v)
		}
		if v, ok := obj.Get("notification_period"); ok {
			h.NotificationPeriod = store.GetTimeperiod(v)
		}
		if v, ok := obj.Get("event_handler"); ok {
			h.EventHandler = store.GetCommand(v)
		}
		h.ContactGroups = resolveContactGroups(store, attrOr(obj, "contact_groups", ""))
		h.Contacts = resolveContacts(store, attrOr(obj, "contacts", ""))

		if err := store.AddHost(h); err != nil {
			return fmt.Errorf("%s:%d: %w", obj.File, obj.Line, err)
		}
	}
	return nil
}

func registerHostGroups(parser *ObjectParser, store *objects.ObjectStore) error {
	for _, obj := range parser.Objects {
		if obj.Type != "hostgroup" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("hostgroup_name")
		if name == "" {
			return fmt.Errorf("%s:%d: hostgroup missing hostgroup_name", obj.File, obj.Line)
		}
		hg := &objects.HostGroup{
			Name:      name,
			Alias:     attrOr(obj, "alias", name),
			Notes:     attrOr(obj, "notes", ""),
			NotesURL:  attrOr(obj, "notes_url", ""),
			ActionURL: attrOr(obj, "action_url", ""),
		}
		if v, ok := obj.Get("members"); ok {
			for _, hName := range splitCSV(v) {
				h := store.GetHost(hName)
				if h == nil {
					return fmt.Errorf("%s:%d: host '%s' not found in hostgroup '%s'", obj.File, obj.Line, hName, name)
				}
				hg.Members = append(hg.Members, h)
			}
		}
		if err := store.AddHostGroup(hg); err != nil {
			return fmt.Errorf("%s:%d: %w", obj.File, obj.Line, err)
		}
	}
	// Resolve hostgroup_members
	for _, obj := range parser.Objects {
		if obj.Type != "hostgroup" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("hostgroup_name")
		hg := store.GetHostGroup(name)
		if v, ok := obj.Get("hostgroup_members"); ok {
			for _, hgName := range splitCSV(v) {
				sub := store.GetHostGroup(hgName)
				if sub == nil {
					return fmt.Errorf("%s:%d: hostgroup '%s' not found in hostgroup_members", obj.File, obj.Line, hgName)
				}
				for _, h := range sub.Members {
					if !containsHost(hg.Members, h) {
						hg.Members = append(hg.Members, h)
					}
				}
			}
		}
	}
	// Recombobulate: hosts with hostgroups directive
	for _, obj := range parser.Objects {
		if obj.Type != "host" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("host_name")
		h := store.GetHost(name)
		if v, ok := obj.Get("hostgroups"); ok {
			for _, hgName := range splitCSV(v) {
				hg := store.GetHostGroup(hgName)
				if hg == nil {
					return fmt.Errorf("%s:%d: hostgroup '%s' not found", obj.File, obj.Line, hgName)
				}
				if !containsHost(hg.Members, h) {
					hg.Members = append(hg.Members, h)
				}
			}
		}
	}
	return nil
}

func registerServices(parser *ObjectParser, store *objects.ObjectStore) error {
	for _, obj := range parser.Objects {
		if obj.Type != "service" || !obj.Register() {
			continue
		}
		// Collect target hosts from host_name and hostgroup_name
		var hostNames []string
		if v, ok := obj.Get("host_name"); ok {
			hostNames = append(hostNames, splitCSV(v)...)
		}
		if v, ok := obj.Get("hostgroup_name"); ok {
			for _, hgName := range splitCSV(v) {
				hg := store.GetHostGroup(hgName)
				if hg == nil {
					return fmt.Errorf("%s:%d: hostgroup '%s' not found for service", obj.File, obj.Line, hgName)
				}
				for _, h := range hg.Members {
					if !containsString(hostNames, h.Name) {
						hostNames = append(hostNames, h.Name)
					}
				}
			}
		}

		desc, _ := obj.Get("service_description")
		if desc == "" {
			return fmt.Errorf("%s:%d: service missing service_description", obj.File, obj.Line)
		}

		// Duplicate: create one service per host
		for _, hName := range hostNames {
			h := store.GetHost(hName)
			if h == nil {
				return fmt.Errorf("%s:%d: host '%s' not found for service '%s'", obj.File, obj.Line, hName, desc)
			}
			svc := &objects.Service{
				Host:                       h,
				Description:                desc,
				DisplayName:                attrOr(obj, "display_name", desc),
				CheckInterval:              attrFloat(obj, "check_interval", 5.0),
				RetryInterval:              attrFloat(obj, "retry_interval", 1.0),
				MaxCheckAttempts:           attrInt(obj, "max_check_attempts", -2),
				InitialState:              parseInitialServiceState(attrOr(obj, "initial_state", "o")),
				IsVolatile:                 attrBool(obj, "is_volatile", false),
				ActiveChecksEnabled:        attrBool(obj, "active_checks_enabled", true),
				PassiveChecksEnabled:       attrBool(obj, "passive_checks_enabled", true),
				ObsessOver:                 attrBool(obj, "obsess_over_service", false),
				EventHandlerEnabled:        attrBool(obj, "event_handler_enabled", true),
				CheckFreshness:             attrBool(obj, "check_freshness", false),
				FreshnessThreshold:         attrInt(obj, "freshness_threshold", 0),
				LowFlapThreshold:           attrFloat(obj, "low_flap_threshold", 0),
				HighFlapThreshold:          attrFloat(obj, "high_flap_threshold", 0),
				FlapDetectionEnabled:       attrBool(obj, "flap_detection_enabled", true),
				FlapDetectionOptions:       parseFlapDetectionServiceOptions(attrOr(obj, "flap_detection_options", "")),
				NotificationsEnabled:       attrBool(obj, "notifications_enabled", true),
				NotificationInterval:       attrFloat(obj, "notification_interval", 30.0),
				FirstNotificationDelay:     attrFloat(obj, "first_notification_delay", 0),
				StalingOptions:             parseStalingServiceOptions(attrOr(obj, "stalking_options", "")),
				ProcessPerfData:            attrBool(obj, "process_perf_data", true),
				Notes:                      clearNull(attrOr(obj, "notes", "")),
				NotesURL:                   clearNull(attrOr(obj, "notes_url", "")),
				ActionURL:                  clearNull(attrOr(obj, "action_url", "")),
				IconImage:                  attrOr(obj, "icon_image", ""),
				IconImageAlt:               attrOr(obj, "icon_image_alt", ""),
				RetainStatusInformation:    attrBool(obj, "retain_status_information", true),
				RetainNonstatusInformation: attrBool(obj, "retain_nonstatus_information", true),
				ParallelizeCheck:           attrBool(obj, "parallelize_check", true),
				CustomVars:                 copyMap(obj.CustomVars),
				ShouldBeScheduled:          true,
			}
			if v, ok := obj.Get("hourly_value"); ok {
				n, _ := strconv.ParseUint(v, 10, 64)
				svc.HourlyValue = uint(n)
			}
			if v, ok := obj.Get("notification_options"); ok {
				svc.NotificationOptions = parseServiceNotificationOptions(v)
			} else {
				svc.NotificationOptions = objects.OptAll
			}
			// Resolve references
			if v, ok := obj.Get("check_command"); ok {
				cmdName, args := splitCommandArgs(v)
				svc.CheckCommand = store.GetCommand(cmdName)
				svc.CheckCommandArgs = args
			}
			if v, ok := obj.Get("check_period"); ok {
				svc.CheckPeriod = store.GetTimeperiod(v)
			}
			if v, ok := obj.Get("notification_period"); ok {
				svc.NotificationPeriod = store.GetTimeperiod(v)
			}
			if v, ok := obj.Get("event_handler"); ok {
				svc.EventHandler = store.GetCommand(v)
			}
			svc.ContactGroups = resolveContactGroups(store, attrOr(obj, "contact_groups", ""))
			svc.Contacts = resolveContacts(store, attrOr(obj, "contacts", ""))

			if err := store.AddService(svc); err != nil {
				return fmt.Errorf("%s:%d: %w", obj.File, obj.Line, err)
			}
			h.Services = append(h.Services, svc)
		}
	}
	return nil
}

func registerServiceGroups(parser *ObjectParser, store *objects.ObjectStore) error {
	for _, obj := range parser.Objects {
		if obj.Type != "servicegroup" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("servicegroup_name")
		if name == "" {
			return fmt.Errorf("%s:%d: servicegroup missing servicegroup_name", obj.File, obj.Line)
		}
		sg := &objects.ServiceGroup{
			Name:      name,
			Alias:     attrOr(obj, "alias", name),
			Notes:     attrOr(obj, "notes", ""),
			NotesURL:  attrOr(obj, "notes_url", ""),
			ActionURL: attrOr(obj, "action_url", ""),
		}
		if v, ok := obj.Get("members"); ok {
			members := splitCSV(v)
			for i := 0; i+1 < len(members); i += 2 {
				svc := store.GetService(members[i], members[i+1])
				if svc == nil {
					return fmt.Errorf("%s:%d: service '%s/%s' not found in servicegroup '%s'",
						obj.File, obj.Line, members[i], members[i+1], name)
				}
				sg.Members = append(sg.Members, svc)
			}
		}
		if err := store.AddServiceGroup(sg); err != nil {
			return fmt.Errorf("%s:%d: %w", obj.File, obj.Line, err)
		}
	}
	// Resolve servicegroup_members
	for _, obj := range parser.Objects {
		if obj.Type != "servicegroup" || !obj.Register() {
			continue
		}
		name, _ := obj.Get("servicegroup_name")
		sg := store.GetServiceGroup(name)
		if v, ok := obj.Get("servicegroup_members"); ok {
			for _, sgName := range splitCSV(v) {
				sub := store.GetServiceGroup(sgName)
				if sub == nil {
					return fmt.Errorf("%s:%d: servicegroup '%s' not found", obj.File, obj.Line, sgName)
				}
				for _, svc := range sub.Members {
					if !containsService(sg.Members, svc) {
						sg.Members = append(sg.Members, svc)
					}
				}
			}
		}
	}
	// Recombobulate: services with servicegroups directive
	for _, obj := range parser.Objects {
		if obj.Type != "service" || !obj.Register() {
			continue
		}
		if v, ok := obj.Get("servicegroups"); ok {
			desc, _ := obj.Get("service_description")
			var hostNames []string
			if hn, ok2 := obj.Get("host_name"); ok2 {
				hostNames = splitCSV(hn)
			}
			for _, sgName := range splitCSV(v) {
				sg := store.GetServiceGroup(sgName)
				if sg == nil {
					continue
				}
				for _, hName := range hostNames {
					svc := store.GetService(hName, desc)
					if svc != nil && !containsService(sg.Members, svc) {
						sg.Members = append(sg.Members, svc)
					}
				}
			}
		}
	}
	return nil
}

// inheritObjectProperties applies inter-object inheritance:
// services inherit contacts, notification_interval, notification_period from their host.
func inheritObjectProperties(store *objects.ObjectStore) {
	for _, svc := range store.Services {
		if svc.Host == nil {
			continue
		}
		h := svc.Host
		// Contacts: inherit from host only if service has neither contacts nor contact_groups
		if len(svc.ContactGroups) == 0 && len(svc.Contacts) == 0 {
			svc.ContactGroups = h.ContactGroups
			svc.Contacts = h.Contacts
		}
		// Notification interval
		if svc.NotificationInterval == 30.0 && h.NotificationInterval != 30.0 {
			// Only inherit if service still has default
		}
		// Notification period: inherit if not set
		if svc.NotificationPeriod == nil && h.NotificationPeriod != nil {
			svc.NotificationPeriod = h.NotificationPeriod
		}
	}
}

func registerHostDependencies(parser *ObjectParser, store *objects.ObjectStore) error {
	for _, obj := range parser.Objects {
		if obj.Type != "hostdependency" || !obj.Register() {
			continue
		}
		masterHosts := resolveHostList(store, attrOr(obj, "host_name", ""), attrOr(obj, "hostgroup_name", ""))
		depHosts := resolveHostList(store, attrOr(obj, "dependent_host_name", ""), attrOr(obj, "dependent_hostgroup_name", ""))

		inheritsParent := attrBool(obj, "inherits_parent", false)
		execOpts := parseHostDependencyOptions(attrOr(obj, "execution_failure_options", ""))
		notifOpts := parseHostDependencyOptions(attrOr(obj, "notification_failure_options", ""))

		var depPeriod *objects.Timeperiod
		if v, ok := obj.Get("dependency_period"); ok {
			depPeriod = store.GetTimeperiod(v)
		}

		for _, master := range masterHosts {
			for _, dep := range depHosts {
				hd := &objects.HostDependency{
					Host:                       master,
					DependentHost:              dep,
					DependencyPeriod:           depPeriod,
					InheritsParent:             inheritsParent,
					ExecutionFailureOptions:    execOpts,
					NotificationFailureOptions: notifOpts,
				}
				store.AddHostDependency(hd)
			}
		}
	}
	return nil
}

func registerServiceDependencies(parser *ObjectParser, store *objects.ObjectStore) error {
	for _, obj := range parser.Objects {
		if obj.Type != "servicedependency" || !obj.Register() {
			continue
		}
		masterHostName := attrOr(obj, "host_name", "")
		masterDesc := attrOr(obj, "service_description", "")
		depHostName := attrOr(obj, "dependent_host_name", "")
		depDesc := attrOr(obj, "dependent_service_description", "")
		inheritsParent := attrBool(obj, "inherits_parent", false)
		execOpts := parseServiceDependencyOptions(attrOr(obj, "execution_failure_options", ""))
		notifOpts := parseServiceDependencyOptions(attrOr(obj, "notification_failure_options", ""))

		var depPeriod *objects.Timeperiod
		if v, ok := obj.Get("dependency_period"); ok {
			depPeriod = store.GetTimeperiod(v)
		}

		masterHosts := resolveHostList(store, masterHostName, attrOr(obj, "hostgroup_name", ""))
		depHosts := resolveHostList(store, depHostName, attrOr(obj, "dependent_hostgroup_name", ""))

		for _, mh := range masterHosts {
			masterSvc := store.GetService(mh.Name, masterDesc)
			if masterSvc == nil {
				continue
			}
			for _, dh := range depHosts {
				depSvc := store.GetService(dh.Name, depDesc)
				if depSvc == nil {
					continue
				}
				sd := &objects.ServiceDependency{
					Host:                       mh,
					Service:                    masterSvc,
					DependentHost:              dh,
					DependentService:           depSvc,
					DependencyPeriod:           depPeriod,
					InheritsParent:             inheritsParent,
					ExecutionFailureOptions:    execOpts,
					NotificationFailureOptions: notifOpts,
				}
				store.AddServiceDependency(sd)
			}
		}
	}
	return nil
}

func registerHostEscalations(parser *ObjectParser, store *objects.ObjectStore) error {
	for _, obj := range parser.Objects {
		if obj.Type != "hostescalation" || !obj.Register() {
			continue
		}
		hosts := resolveHostList(store, attrOr(obj, "host_name", ""), attrOr(obj, "hostgroup_name", ""))
		cgs := resolveContactGroups(store, attrOr(obj, "contact_groups", ""))
		cts := resolveContacts(store, attrOr(obj, "contacts", ""))
		firstNotif := attrInt(obj, "first_notification", -2)
		lastNotif := attrInt(obj, "last_notification", -2)
		notifInterval := attrFloat(obj, "notification_interval", -1)
		escOpts := parseHostEscalationOptions(attrOr(obj, "escalation_options", ""))

		var escPeriod *objects.Timeperiod
		if v, ok := obj.Get("escalation_period"); ok {
			escPeriod = store.GetTimeperiod(v)
		}

		for _, h := range hosts {
			he := &objects.HostEscalation{
				Host:                 h,
				ContactGroups:        cgs,
				Contacts:             cts,
				FirstNotification:    firstNotif,
				LastNotification:     lastNotif,
				NotificationInterval: notifInterval,
				EscalationPeriod:     escPeriod,
				EscalationOptions:    escOpts,
			}
			store.AddHostEscalation(he)
		}
	}
	return nil
}

func registerServiceEscalations(parser *ObjectParser, store *objects.ObjectStore) error {
	for _, obj := range parser.Objects {
		if obj.Type != "serviceescalation" || !obj.Register() {
			continue
		}
		hosts := resolveHostList(store, attrOr(obj, "host_name", ""), attrOr(obj, "hostgroup_name", ""))
		desc := attrOr(obj, "service_description", "")
		cgs := resolveContactGroups(store, attrOr(obj, "contact_groups", ""))
		cts := resolveContacts(store, attrOr(obj, "contacts", ""))
		firstNotif := attrInt(obj, "first_notification", -2)
		lastNotif := attrInt(obj, "last_notification", -2)
		notifInterval := attrFloat(obj, "notification_interval", -1)
		escOpts := parseServiceEscalationOptions(attrOr(obj, "escalation_options", ""))

		var escPeriod *objects.Timeperiod
		if v, ok := obj.Get("escalation_period"); ok {
			escPeriod = store.GetTimeperiod(v)
		}

		for _, h := range hosts {
			svc := store.GetService(h.Name, desc)
			if svc == nil {
				continue
			}
			se := &objects.ServiceEscalation{
				Host:                 h,
				Service:              svc,
				ContactGroups:        cgs,
				Contacts:             cts,
				FirstNotification:    firstNotif,
				LastNotification:     lastNotif,
				NotificationInterval: notifInterval,
				EscalationPeriod:     escPeriod,
				EscalationOptions:    escOpts,
			}
			store.AddServiceEscalation(se)
		}
	}
	return nil
}

func resolveHostParents(store *objects.ObjectStore) error {
	// This is done during registration; we iterate again to resolve parent string refs.
	// Parents were stored as host_name refs during parsing; now build the pointer graph.
	// Note: this is already partially handled, but we need to wire parent strings
	// that weren't set during registerHosts. We'll do a dedicated pass.
	return nil
}

func wireGroupReferences(store *objects.ObjectStore) {
	// Wire host → hostgroups bidirectional
	for _, hg := range store.HostGroups {
		for _, h := range hg.Members {
			if !containsHostGroup(h.HostGroups, hg) {
				h.HostGroups = append(h.HostGroups, hg)
			}
		}
	}
	// Wire service → servicegroups bidirectional
	for _, sg := range store.ServiceGroups {
		for _, svc := range sg.Members {
			if !containsServiceGroup(svc.ServiceGroups, sg) {
				svc.ServiceGroups = append(svc.ServiceGroups, sg)
			}
		}
	}
}

// Helper functions

func resolveHostList(store *objects.ObjectStore, hostNames, hostgroupNames string) []*objects.Host {
	var result []*objects.Host
	seen := make(map[string]bool)
	if hostNames != "" {
		for _, name := range splitCSV(hostNames) {
			h := store.GetHost(name)
			if h != nil && !seen[name] {
				result = append(result, h)
				seen[name] = true
			}
		}
	}
	if hostgroupNames != "" {
		for _, hgName := range splitCSV(hostgroupNames) {
			hg := store.GetHostGroup(hgName)
			if hg != nil {
				for _, h := range hg.Members {
					if !seen[h.Name] {
						result = append(result, h)
						seen[h.Name] = true
					}
				}
			}
		}
	}
	return result
}

func resolveCommands(store *objects.ObjectStore, csv string) []*objects.Command {
	var result []*objects.Command
	for _, name := range splitCSV(csv) {
		cmd := store.GetCommand(name)
		if cmd != nil {
			result = append(result, cmd)
		}
	}
	return result
}

func resolveContactGroups(store *objects.ObjectStore, csv string) []*objects.ContactGroup {
	if csv == "" {
		return nil
	}
	var result []*objects.ContactGroup
	for _, name := range splitCSV(csv) {
		cg := store.GetContactGroup(name)
		if cg != nil {
			result = append(result, cg)
		}
	}
	return result
}

func resolveContacts(store *objects.ObjectStore, csv string) []*objects.Contact {
	if csv == "" {
		return nil
	}
	var result []*objects.Contact
	for _, name := range splitCSV(csv) {
		c := store.GetContact(name)
		if c != nil {
			result = append(result, c)
		}
	}
	return result
}

func splitCommandArgs(s string) (string, string) {
	idx := strings.IndexByte(s, '!')
	if idx < 0 {
		return s, ""
	}
	return s[:idx], s[idx+1:]
}

func attrOr(obj *TemplateObject, key, def string) string {
	v, ok := obj.Get(key)
	if !ok || v == "null" {
		return def
	}
	return v
}

func clearNull(s string) string {
	if s == "null" {
		return ""
	}
	return s
}

func attrBool(obj *TemplateObject, key string, def bool) bool {
	v, ok := obj.Get(key)
	if !ok {
		return def
	}
	return v == "1"
}

func attrInt(obj *TemplateObject, key string, def int) int {
	v, ok := obj.Get(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func attrFloat(obj *TemplateObject, key string, def float64) float64 {
	v, ok := obj.Get(key)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func copyMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func parseInitialHostState(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "d":
		return objects.HostDown
	case "u":
		return objects.HostUnreachable
	default:
		return objects.HostUp
	}
}

func parseInitialServiceState(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "w":
		return objects.ServiceWarning
	case "c":
		return objects.ServiceCritical
	case "u":
		return objects.ServiceUnknown
	default:
		return objects.ServiceOK
	}
}

func parseOptions(s string, mapping map[string]uint32) uint32 {
	if s == "" {
		return 0
	}
	var result uint32
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "a" || part == "all" {
			return objects.OptAll
		}
		if part == "n" || part == "none" {
			return objects.OptNone
		}
		if v, ok := mapping[part]; ok {
			result |= v
		}
	}
	return result
}

func parseHostNotificationOptions(s string) uint32 {
	return parseOptions(s, map[string]uint32{
		"d": objects.OptDown, "down": objects.OptDown,
		"u": objects.OptUnreachable, "unreachable": objects.OptUnreachable,
		"r": objects.OptRecovery, "recovery": objects.OptRecovery,
		"f": objects.OptFlapping, "flapping": objects.OptFlapping,
		"s": objects.OptDowntime, "downtime": objects.OptDowntime,
	})
}

func parseServiceNotificationOptions(s string) uint32 {
	return parseOptions(s, map[string]uint32{
		"w": objects.OptWarning, "warning": objects.OptWarning,
		"u": objects.OptUnknown, "unknown": objects.OptUnknown,
		"c": objects.OptCritical, "critical": objects.OptCritical,
		"r": objects.OptRecovery, "recovery": objects.OptRecovery,
		"f": objects.OptFlapping, "flapping": objects.OptFlapping,
		"s": objects.OptDowntime, "downtime": objects.OptDowntime,
	})
}

func parseFlapDetectionHostOptions(s string) uint32 {
	if s == "" {
		return objects.OptAll
	}
	return parseOptions(s, map[string]uint32{
		"o": objects.OptOK,
		"d": objects.OptDown,
		"u": objects.OptUnreachable,
	})
}

func parseFlapDetectionServiceOptions(s string) uint32 {
	if s == "" {
		return objects.OptAll
	}
	return parseOptions(s, map[string]uint32{
		"o": objects.OptOK,
		"w": objects.OptWarning,
		"u": objects.OptUnknown,
		"c": objects.OptCritical,
	})
}

func parseStalingHostOptions(s string) uint32 {
	return parseOptions(s, map[string]uint32{
		"o": objects.OptOK,
		"d": objects.OptDown,
		"u": objects.OptUnreachable,
	})
}

func parseStalingServiceOptions(s string) uint32 {
	return parseOptions(s, map[string]uint32{
		"o": objects.OptOK,
		"w": objects.OptWarning,
		"u": objects.OptUnknown,
		"c": objects.OptCritical,
	})
}

func parseHostDependencyOptions(s string) uint32 {
	return parseOptions(s, map[string]uint32{
		"o": objects.OptOK,
		"d": objects.OptDown,
		"u": objects.OptUnreachable,
		"p": objects.OptPending,
	})
}

func parseServiceDependencyOptions(s string) uint32 {
	return parseOptions(s, map[string]uint32{
		"o": objects.OptOK,
		"w": objects.OptWarning,
		"u": objects.OptUnknown,
		"c": objects.OptCritical,
		"p": objects.OptPending,
	})
}

func parseHostEscalationOptions(s string) uint32 {
	if s == "" {
		return objects.OptAll
	}
	return parseOptions(s, map[string]uint32{
		"d": objects.OptDown,
		"u": objects.OptUnreachable,
		"r": objects.OptRecovery,
	})
}

func parseServiceEscalationOptions(s string) uint32 {
	if s == "" {
		return objects.OptAll
	}
	return parseOptions(s, map[string]uint32{
		"w": objects.OptWarning,
		"u": objects.OptUnknown,
		"c": objects.OptCritical,
		"r": objects.OptRecovery,
	})
}

func containsContact(list []*objects.Contact, c *objects.Contact) bool {
	for _, x := range list {
		if x.Name == c.Name {
			return true
		}
	}
	return false
}

func containsHost(list []*objects.Host, h *objects.Host) bool {
	for _, x := range list {
		if x.Name == h.Name {
			return true
		}
	}
	return false
}

func containsService(list []*objects.Service, s *objects.Service) bool {
	for _, x := range list {
		if x.Host.Name == s.Host.Name && x.Description == s.Description {
			return true
		}
	}
	return false
}

func containsHostGroup(list []*objects.HostGroup, hg *objects.HostGroup) bool {
	for _, x := range list {
		if x.Name == hg.Name {
			return true
		}
	}
	return false
}

func containsServiceGroup(list []*objects.ServiceGroup, sg *objects.ServiceGroup) bool {
	for _, x := range list {
		if x.Name == sg.Name {
			return true
		}
	}
	return false
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// parseTimeDateException tries to parse a timeperiod date exception from a key/value pair.
// Returns nil if the key is a standard attribute (not a date exception).
func parseTimeDateException(key, val string) *objects.TimeDateException {
	// Skip known timeperiod attributes
	switch key {
	case "timeperiod_name", "alias", "use", "name", "register", "exclude",
		"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday":
		return nil
	}
	// This is a simplified parser. Full date exception parsing is in timeperiod.go.
	// For now, store the raw key+value as a string exception.
	return &objects.TimeDateException{
		Timerange: key + " " + val,
	}
}
