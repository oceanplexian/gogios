package objects

import "time"

// State constants
const (
	HostUp          = 0
	HostDown        = 1
	HostUnreachable = 2

	ServiceOK       = 0
	ServiceWarning  = 1
	ServiceCritical = 2
	ServiceUnknown  = 3

	StateTypeSoft = 0
	StateTypeHard = 1

	MaxContactAddresses    = 6
	MaxStateHistoryEntries = 21

	AckNone   = 0
	AckNormal = 1
	AckSticky = 2
)

// Notification types
const (
	NotificationNormal            = 0
	NotificationAcknowledgement   = 1
	NotificationFlappingStart     = 2
	NotificationFlappingStop      = 3
	NotificationFlappingDisabled  = 4
	NotificationDowntimeStart     = 5
	NotificationDowntimeEnd       = 6
	NotificationDowntimeCancelled = 7
	NotificationCustom            = 8
)

// Notification option flags
const (
	NotificationOptionNone      = 0
	NotificationOptionBroadcast = 1
	NotificationOptionForced    = 2
	NotificationOptionIncrement = 4
)

// Dependency types
const (
	NotificationDependency = 1
	ExecutionDependency    = 2
)

// Comment entry types
const (
	UserCommentEntry           = 1
	DowntimeCommentEntry       = 2
	FlappingCommentEntry       = 3
	AcknowledgementCommentEntry = 4
)

// Comment types
const (
	HostCommentType    = 1
	ServiceCommentType = 2
)

// Downtime types
const (
	HostDowntimeType    = 1
	ServiceDowntimeType = 2
)

// Log rotation methods
const (
	LogRotationNone    = 0
	LogRotationHourly  = 1
	LogRotationDaily   = 2
	LogRotationWeekly  = 3
	LogRotationMonthly = 4
)

// Perfdata file modes
const (
	PerfdataFileAppend = 0
	PerfdataFileWrite  = 1
	PerfdataFilePipe   = 2
)

// Notification/flap detection option bitmasks
const (
	OptDown        uint32 = 1 << iota // d
	OptUnreachable                    // u
	OptRecovery                       // r
	OptFlapping                       // f
	OptDowntime                       // s
	OptWarning                        // w
	OptCritical                       // c
	OptUnknown                        // k (using k to avoid conflict with u)
	OptOK                             // o
	OptPending                        // p
	OptNone        uint32 = 0
	OptAll         uint32 = 0xFFFF
)

type Command struct {
	Name        string
	CommandLine string
}

type Timeperiod struct {
	Name       string
	Alias      string
	Ranges     [7]string // sunday=0 through saturday=6
	Exceptions []TimeDateException
	Exclusions []*Timeperiod
	CustomVars map[string]string
}

type TimeDateException struct {
	Type      int // 0=calendar, 1=month_date, 2=month_day, 3=month_weekday, 4=weekday, 5=daterange
	Year      int
	Month     int
	MonthDay  int
	Weekday   int // 0=sunday
	Skip      int
	Year2     int
	Month2    int
	MonthDay2 int
	Weekday2  int
	Timerange string
}

type Contact struct {
	Name                          string
	Alias                         string
	Email                         string
	Pager                         string
	Addresses                     [MaxContactAddresses]string
	HostNotificationPeriod        *Timeperiod
	ServiceNotificationPeriod     *Timeperiod
	HostNotificationCommands      []*Command
	ServiceNotificationCommands   []*Command
	HostNotificationOptions       uint32
	ServiceNotificationOptions    uint32
	HostNotificationsEnabled      bool
	ServiceNotificationsEnabled   bool
	CanSubmitCommands             bool
	RetainStatusInformation       bool
	RetainNonstatusInformation    bool
	MinimumImportance             uint
	ContactGroups                 []*ContactGroup
	CustomVars                    map[string]string
	// Runtime
	LastHostNotification          time.Time
	LastServiceNotification       time.Time
	ModifiedAttributes            uint64
	ModifiedHostAttributes        uint64
	ModifiedServiceAttributes     uint64
}

// GlobalState holds global runtime state for the Nagios process.
type GlobalState struct {
	EnableNotifications            bool
	ExecuteServiceChecks           bool
	ExecuteHostChecks              bool
	AcceptPassiveServiceChecks     bool
	AcceptPassiveHostChecks        bool
	EnableEventHandlers            bool
	ObsessOverServices             bool
	ObsessOverHosts                bool
	CheckServiceFreshness          bool
	CheckHostFreshness             bool
	EnableFlapDetection            bool
	ProcessPerformanceData         bool
	GlobalHostEventHandler         string
	GlobalServiceEventHandler      string
	NextCommentID                  uint64
	NextDowntimeID                 uint64
	NextEventID                    uint64
	NextProblemID                  uint64
	NextNotificationID             uint64
	ProgramStart                   time.Time
	PID                            int
	DaemonMode                     bool
	IntervalLength                 int
	ModifiedHostAttributes         uint64
	ModifiedServiceAttributes      uint64
	SoftStateDependencies          bool
	LogNotifications               bool
	LogServiceRetries              bool
	LogEventHandlers               bool
	LogExternalCommands            bool
	LogPassiveChecks               bool
	LogInitialStates               bool
	LogFile                        string
	LogArchivePath                 string
	LogRotationMethod              int
	CommandFile                    string
	StatusFile                     string
	TempFile                       string
	RetentionFile                  string
	RetainStateInformation         bool
	HostPerfdataCommand            string
	ServicePerfdataCommand         string
	HostPerfdataFile               string
	ServicePerfdataFile            string
	HostPerfdataFileTemplate       string
	ServicePerfdataFileTemplate    string
	HostPerfdataFileMode           int
	ServicePerfdataFileMode        int
	HostPerfdataFileProcessingCommand    string
	ServicePerfdataFileProcessingCommand string
	HostPerfdataFileProcessingInterval   int
	ServicePerfdataFileProcessingInterval int
	HostPerfdataProcessEmptyResults      bool
	ServicePerfdataProcessEmptyResults   bool
}

type ContactGroup struct {
	Name    string
	Alias   string
	Members []*Contact
}

type Host struct {
	// Config
	Name                       string
	DisplayName                string
	Alias                      string
	Address                    string
	Parents                    []*Host
	Children                   []*Host
	HostGroups                 []*HostGroup
	Services                   []*Service
	CheckCommand               *Command
	CheckCommandArgs           string
	CheckPeriod                *Timeperiod
	CheckInterval              float64
	RetryInterval              float64
	MaxCheckAttempts           int
	InitialState               int
	ActiveChecksEnabled        bool
	PassiveChecksEnabled       bool
	ObsessOver                 bool
	EventHandler               *Command
	EventHandlerEnabled        bool
	CheckFreshness             bool
	FreshnessThreshold         int
	LowFlapThreshold           float64
	HighFlapThreshold          float64
	FlapDetectionEnabled       bool
	FlapDetectionOptions       uint32
	ContactGroups              []*ContactGroup
	Contacts                   []*Contact
	NotificationOptions        uint32
	NotificationsEnabled       bool
	NotificationPeriod         *Timeperiod
	NotificationInterval       float64
	FirstNotificationDelay     float64
	StalingOptions             uint32
	ProcessPerfData            bool
	Notes                      string
	NotesURL                   string
	ActionURL                  string
	IconImage                  string
	IconImageAlt               string
	VRMLImage                  string
	StatusmapImage             string
	X2D, Y2D                   int
	Have2DCoords               bool
	X3D, Y3D, Z3D             float64
	Have3DCoords               bool
	RetainStatusInformation    bool
	RetainNonstatusInformation bool
	HourlyValue                uint
	CustomVars                 map[string]string

	// Runtime state
	CurrentState        int
	LastState           int
	LastHardState       int
	StateType           int
	CurrentAttempt      int
	HasBeenChecked      bool
	IsExecuting         bool
	IsFlapping          bool
	PluginOutput        string
	LongPluginOutput    string
	PerfData            string
	LastCheck           time.Time
	NextCheck           time.Time
	LastStateChange     time.Time
	LastHardStateChange time.Time
	LastTimeUp          time.Time
	LastTimeDown        time.Time
	LastTimeUnreachable time.Time
	ShouldBeScheduled   bool
	CheckOptions        int
	Latency             float64
	ExecutionTime       float64

	// Flap detection state
	StateHistory      [MaxStateHistoryEntries]int
	StateHistoryIndex int
	PercentStateChange float64

	// Notification state
	CurrentNotificationNumber int
	CurrentNotificationID     uint64
	LastNotification          time.Time
	NextNotification          time.Time
	NotifiedOn                uint32
	NoMoreNotifications       bool
	ProblemAcknowledged       bool
	AckType                   int
	ScheduledDowntimeDepth    int
	PendingFlexDowntime       int
	CheckFlapRecoveryNotif    bool
	FirstProblemTime          time.Time
	ModifiedAttributes        uint64

	CurrentEventID   uint64
	LastEventID      uint64
	CurrentProblemID uint64
	LastProblemID    uint64

	// Escalations and Dependencies
	Escalations []*HostEscalation
	NotifyDeps  []*HostDependency
	ExecDeps    []*HostDependency

	// Freshness
	IsBeingFreshened bool

	// Notification-related config
	CheckType int

	// Dynamic NRDP objects
	Dynamic  bool      // true if auto-created via NRDP, eligible for TTL pruning
	LastSeen time.Time // last time a passive check was received (for TTL pruning)
}

type HostGroup struct {
	Name      string
	Alias     string
	Members   []*Host
	Notes     string
	NotesURL  string
	ActionURL string
}

type Service struct {
	// Config
	Host                       *Host
	Description                string
	DisplayName                string
	ServiceGroups              []*ServiceGroup
	CheckCommand               *Command
	CheckCommandArgs           string
	CheckPeriod                *Timeperiod
	CheckInterval              float64
	RetryInterval              float64
	MaxCheckAttempts           int
	InitialState               int
	IsVolatile                 bool
	ActiveChecksEnabled        bool
	PassiveChecksEnabled       bool
	ObsessOver                 bool
	EventHandler               *Command
	EventHandlerEnabled        bool
	CheckFreshness             bool
	FreshnessThreshold         int
	LowFlapThreshold           float64
	HighFlapThreshold          float64
	FlapDetectionEnabled       bool
	FlapDetectionOptions       uint32
	ContactGroups              []*ContactGroup
	Contacts                   []*Contact
	NotificationOptions        uint32
	NotificationsEnabled       bool
	NotificationPeriod         *Timeperiod
	NotificationInterval       float64
	FirstNotificationDelay     float64
	StalingOptions             uint32
	ProcessPerfData            bool
	Notes                      string
	NotesURL                   string
	ActionURL                  string
	IconImage                  string
	IconImageAlt               string
	RetainStatusInformation    bool
	RetainNonstatusInformation bool
	HourlyValue                uint
	ParallelizeCheck           bool
	CustomVars                 map[string]string

	// Runtime state
	CurrentState        int
	LastState           int
	LastHardState       int
	StateType           int
	CurrentAttempt      int
	HasBeenChecked      bool
	IsExecuting         bool
	IsFlapping          bool
	PluginOutput        string
	LongPluginOutput    string
	PerfData            string
	LastCheck           time.Time
	NextCheck           time.Time
	LastStateChange     time.Time
	LastHardStateChange time.Time
	LastTimeOK          time.Time
	LastTimeWarning     time.Time
	LastTimeCritical    time.Time
	LastTimeUnknown     time.Time
	ShouldBeScheduled   bool
	CheckOptions        int
	Latency             float64
	ExecutionTime       float64

	// Flap detection state
	StateHistory      [MaxStateHistoryEntries]int
	StateHistoryIndex int
	PercentStateChange float64

	// Notification state
	CurrentNotificationNumber int
	CurrentNotificationID     uint64
	LastNotification          time.Time
	NextNotification          time.Time
	NotifiedOn                uint32
	NoMoreNotifications       bool
	ProblemAcknowledged       bool
	AckType                   int
	ScheduledDowntimeDepth    int
	PendingFlexDowntime       int
	HostProblemAtLastCheck    bool
	CheckFlapRecoveryNotif    bool
	FirstProblemTime          time.Time
	ModifiedAttributes        uint64

	CurrentEventID   uint64
	LastEventID      uint64
	CurrentProblemID uint64
	LastProblemID    uint64

	// Escalations and Dependencies
	Escalations []*ServiceEscalation
	NotifyDeps  []*ServiceDependency
	ExecDeps    []*ServiceDependency
	ServiceParents []*Service

	// Freshness
	IsBeingFreshened bool

	CheckType int

	// Dynamic NRDP objects
	Dynamic  bool      // true if auto-created via NRDP, eligible for TTL pruning
	LastSeen time.Time // last time a passive check was received (for TTL pruning)
}

// CheckResult carries the result of a plugin execution back to the main loop.
type CheckResult struct {
	HostName           string
	ServiceDescription string // empty for host checks
	CheckType          int    // CheckTypeActive or CheckTypePassive
	ReturnCode         int
	Output             string
	StartTime          time.Time
	FinishTime         time.Time
	EarlyTimeout       bool
	ExitedOK           bool
	Latency            float64
	ExecutionTime      float64
	CheckOptions       int
	DynamicRegister    bool // NRDP: auto-create host/service in scheduler goroutine
}

// Check option flags
const (
	CheckOptionNone             = 0
	CheckOptionForceExecution   = 1 << 0
	CheckOptionFreshnessCheck   = 1 << 1
	CheckOptionOrphanCheck      = 1 << 2
	CheckOptionDependencyCheck  = 1 << 3
)

// CheckTypeActive / CheckTypePassive
const (
	CheckTypeActive  = 0
	CheckTypePassive = 1
)

// Config holds global configuration relevant to the check engine.
type Config struct {
	IntervalLength                int     // default 60
	ServiceInterCheckDelayMethod  int     // 0=NONE, 1=DUMB, 2=SMART, 3=USER
	HostInterCheckDelayMethod     int
	ServiceInterCheckDelay        float64 // calculated or user-set
	HostInterCheckDelay           float64
	ServiceInterleaveMethod       int     // ILF_SMART=2
	ServiceInterleaveFactor       int
	MaxServiceCheckSpread         int // minutes, default 30
	MaxHostCheckSpread            int
	MaxParallelServiceChecks      int // 0 = unlimited
	ServiceCheckTimeout           int // seconds
	HostCheckTimeout              int // seconds
	ExecuteServiceChecks          bool
	ExecuteHostChecks             bool
	CheckReaperInterval           int
	ServiceFreshnessCheckInterval int
	HostFreshnessCheckInterval    int
	CheckServiceFreshness         bool
	CheckHostFreshness            bool
	StatusUpdateInterval          int
	RetentionUpdateInterval       int // minutes
	LogRotationInterval           int // 0=none
	AutoReschedulingInterval      int
	AutoReschedulingEnabled       bool
	AdditionalFreshnessLatency    int
	UseAggressiveHostChecking     bool
	TranslatePassiveHostChecks    bool
	ServiceCheckTimeoutState      int // default ServiceCritical
	HostDownDisableServiceChecks  bool
	AvgServiceExecutionTime       float64
	UserMacros                    [256]string
	OrphanCheckInterval           int // default 60
}

// DefaultConfig returns a Config with Nagios 4.1.1 defaults.
func DefaultConfig() *Config {
	return &Config{
		IntervalLength:                60,
		ServiceInterCheckDelayMethod:  2,
		HostInterCheckDelayMethod:     2,
		ServiceInterleaveMethod:       2,
		MaxServiceCheckSpread:         30,
		MaxHostCheckSpread:            30,
		ServiceCheckTimeout:           60,
		HostCheckTimeout:              30,
		ExecuteServiceChecks:          true,
		ExecuteHostChecks:             true,
		CheckReaperInterval:           10,
		ServiceFreshnessCheckInterval: 60,
		HostFreshnessCheckInterval:    60,
		StatusUpdateInterval:          60,
		RetentionUpdateInterval:       60,
		AdditionalFreshnessLatency:    15,
		ServiceCheckTimeoutState:      ServiceCritical,
		AvgServiceExecutionTime:       2.0,
		OrphanCheckInterval:           60,
	}
}

type ServiceGroup struct {
	Name      string
	Alias     string
	Members   []*Service
	Notes     string
	NotesURL  string
	ActionURL string
}

type HostDependency struct {
	DependentHost              *Host
	Host                       *Host
	DependencyPeriod           *Timeperiod
	InheritsParent             bool
	ExecutionFailureOptions    uint32
	NotificationFailureOptions uint32
}

type ServiceDependency struct {
	DependentHost              *Host
	DependentService           *Service
	Host                       *Host
	Service                    *Service
	DependencyPeriod           *Timeperiod
	InheritsParent             bool
	ExecutionFailureOptions    uint32
	NotificationFailureOptions uint32
}

type HostEscalation struct {
	Host                 *Host
	ContactGroups        []*ContactGroup
	Contacts             []*Contact
	FirstNotification    int
	LastNotification     int
	NotificationInterval float64
	EscalationPeriod     *Timeperiod
	EscalationOptions    uint32
}

type ServiceEscalation struct {
	Host                 *Host
	Service              *Service
	ContactGroups        []*ContactGroup
	Contacts             []*Contact
	FirstNotification    int
	LastNotification     int
	NotificationInterval float64
	EscalationPeriod     *Timeperiod
	EscalationOptions    uint32
}

// NotificationTypeName returns the $NOTIFICATIONTYPE$ macro string.
func NotificationTypeName(ntype, state int, isHost bool) string {
	switch ntype {
	case NotificationAcknowledgement:
		return "ACKNOWLEDGEMENT"
	case NotificationFlappingStart:
		return "FLAPPINGSTART"
	case NotificationFlappingStop:
		return "FLAPPINGSTOP"
	case NotificationFlappingDisabled:
		return "FLAPPINGDISABLED"
	case NotificationDowntimeStart:
		return "DOWNTIMESTART"
	case NotificationDowntimeEnd:
		return "DOWNTIMEEND"
	case NotificationDowntimeCancelled:
		return "DOWNTIMECANCELLED"
	case NotificationCustom:
		return "CUSTOM"
	default:
		if isHost {
			if state == HostUp {
				return "RECOVERY"
			}
			return "PROBLEM"
		}
		if state == ServiceOK {
			return "RECOVERY"
		}
		return "PROBLEM"
	}
}

// HostStateName returns the display name for a host state.
func HostStateName(state int) string {
	switch state {
	case HostUp:
		return "UP"
	case HostDown:
		return "DOWN"
	case HostUnreachable:
		return "UNREACHABLE"
	default:
		return "UNKNOWN"
	}
}

// ServiceStateName returns the display name for a service state.
func ServiceStateName(state int) string {
	switch state {
	case ServiceOK:
		return "OK"
	case ServiceWarning:
		return "WARNING"
	case ServiceCritical:
		return "CRITICAL"
	case ServiceUnknown:
		return "UNKNOWN"
	default:
		return "UNKNOWN"
	}
}

// StateTypeName returns "HARD" or "SOFT".
func StateTypeName(st int) string {
	if st == StateTypeHard {
		return "HARD"
	}
	return "SOFT"
}

// StateMatchesHostOptions checks if a host state matches notification options.
func StateMatchesHostOptions(state int, opts uint32) bool {
	switch state {
	case HostDown:
		return opts&OptDown != 0
	case HostUnreachable:
		return opts&OptUnreachable != 0
	case HostUp:
		return opts&OptRecovery != 0
	}
	return false
}

// StateMatchesSvcOptions checks if a service state matches notification options.
func StateMatchesSvcOptions(state int, opts uint32) bool {
	switch state {
	case ServiceWarning:
		return opts&OptWarning != 0
	case ServiceCritical:
		return opts&OptCritical != 0
	case ServiceUnknown:
		return opts&OptUnknown != 0
	case ServiceOK:
		return opts&OptRecovery != 0
	}
	return false
}

// InTimeperiod checks if a time falls within a timeperiod. Returns true if tp is nil.
func InTimeperiod(tp *Timeperiod, t time.Time) bool {
	if tp == nil {
		return true
	}
	// Timeperiod logic will be implemented by config parser.
	// For now, all times are valid if ranges are empty (24x7 default).
	return true
}
