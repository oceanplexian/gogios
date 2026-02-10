package downtime

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// Downtime represents a scheduled downtime entry.
type Downtime struct {
	Type                      int // HostDowntimeType or ServiceDowntimeType
	HostName                  string
	ServiceDescription        string
	EntryTime                 time.Time
	StartTime                 time.Time
	FlexDowntimeStart         time.Time
	EndTime                   time.Time
	Fixed                     bool
	TriggeredBy               uint64 // ID of triggering downtime, 0=none
	Duration                  time.Duration
	DowntimeID                uint64
	IsInEffect                bool
	StartNotificationSent     bool
	Author                    string
	Comment                   string
	CommentID                 uint64
	IncrementedPendingDowntime bool
}

// Logger is the interface for downtime log events.
type Logger interface {
	Log(format string, args ...interface{})
}

// Notifier is the interface for sending downtime notifications.
type Notifier interface {
	SendHostNotification(hostName string, ntype int, author, data string, options int)
	SendServiceNotification(hostName, svcDesc string, ntype int, author, data string, options int)
}

// DowntimeManager manages all scheduled downtimes.
type DowntimeManager struct {
	mu        sync.RWMutex
	downtimes map[uint64]*Downtime
	nextID    atomic.Uint64
	comments  *CommentManager
	store     *objects.ObjectStore
	logger    Logger
	notifier  Notifier
}

// NewDowntimeManager creates a new downtime manager.
func NewDowntimeManager(startID uint64, comments *CommentManager, store *objects.ObjectStore) *DowntimeManager {
	dm := &DowntimeManager{
		downtimes: make(map[uint64]*Downtime),
		comments:  comments,
		store:     store,
	}
	dm.nextID.Store(startID)
	return dm
}

// SetLogger sets the logger.
func (dm *DowntimeManager) SetLogger(l Logger) { dm.logger = l }

// SetNotifier sets the notifier.
func (dm *DowntimeManager) SetNotifier(n Notifier) { dm.notifier = n }

func (dm *DowntimeManager) log(format string, args ...interface{}) {
	if dm.logger != nil {
		dm.logger.Log(format, args...)
	}
}

// Schedule adds a new downtime entry and returns its ID.
func (dm *DowntimeManager) Schedule(d *Downtime) uint64 {
	id := dm.nextID.Add(1) - 1
	d.DowntimeID = id
	if d.EntryTime.IsZero() {
		d.EntryTime = time.Now()
	}

	// Add downtime comment
	commentType := objects.HostCommentType
	if d.Type == objects.ServiceDowntimeType {
		commentType = objects.ServiceCommentType
	}
	commentText := fmt.Sprintf("This %s has been scheduled for fixed downtime from %s to %s.",
		downtimeTypeName(d.Type), d.StartTime.Format(time.RFC3339), d.EndTime.Format(time.RFC3339))
	if !d.Fixed {
		commentText = fmt.Sprintf("This %s has been scheduled for flexible downtime starting between %s and %s and lasting for %s.",
			downtimeTypeName(d.Type), d.StartTime.Format(time.RFC3339), d.EndTime.Format(time.RFC3339), d.Duration)
	}
	c := &Comment{
		CommentType:        commentType,
		EntryType:          objects.DowntimeCommentEntry,
		Source:             0,
		Persistent:         false,
		HostName:           d.HostName,
		ServiceDescription: d.ServiceDescription,
		Author:             d.Author,
		Data:               commentText,
	}
	d.CommentID = dm.comments.Add(c)

	dm.mu.Lock()
	dm.downtimes[id] = d
	dm.mu.Unlock()

	// For flexible downtimes, increment pending counter
	if !d.Fixed && d.TriggeredBy == 0 {
		dm.incrementPending(d)
	}

	return id
}

// ScheduleWithID adds a downtime with a specific ID (for retention restore).
func (dm *DowntimeManager) ScheduleWithID(d *Downtime) {
	dm.mu.Lock()
	dm.downtimes[d.DowntimeID] = d
	dm.mu.Unlock()
	for {
		cur := dm.nextID.Load()
		if d.DowntimeID < cur {
			break
		}
		if dm.nextID.CompareAndSwap(cur, d.DowntimeID+1) {
			break
		}
	}
}

// Unschedule cancels a downtime.
func (dm *DowntimeManager) Unschedule(id uint64) {
	dm.mu.Lock()
	d, ok := dm.downtimes[id]
	if !ok {
		dm.mu.Unlock()
		return
	}
	dm.mu.Unlock()

	if d.IncrementedPendingDowntime {
		dm.decrementPending(d)
	}

	if d.IsInEffect {
		dm.stopDowntime(d, true)
	}

	// Delete comment
	if d.CommentID > 0 {
		dm.comments.Delete(d.CommentID)
	}

	dm.mu.Lock()
	delete(dm.downtimes, id)
	dm.mu.Unlock()

	// Recursively unschedule triggered downtimes
	dm.unscheduleTriggered(id)
}

func (dm *DowntimeManager) unscheduleTriggered(triggerID uint64) {
	dm.mu.RLock()
	var triggered []uint64
	for id, d := range dm.downtimes {
		if d.TriggeredBy == triggerID {
			triggered = append(triggered, id)
		}
	}
	dm.mu.RUnlock()
	for _, id := range triggered {
		dm.Unschedule(id)
	}
}

// HandleStart processes a downtime start event.
func (dm *DowntimeManager) HandleStart(id uint64) {
	dm.mu.RLock()
	d, ok := dm.downtimes[id]
	dm.mu.RUnlock()
	if !ok || d.IsInEffect {
		return
	}

	d.IsInEffect = true

	if d.Type == objects.HostDowntimeType {
		hst := dm.store.GetHost(d.HostName)
		if hst != nil {
			if hst.ScheduledDowntimeDepth == 0 {
				dm.log("HOST DOWNTIME ALERT: %s;STARTED; %s has entered a period of scheduled downtime", d.HostName, d.HostName)
				if !d.StartNotificationSent && dm.notifier != nil {
					dm.notifier.SendHostNotification(d.HostName, objects.NotificationDowntimeStart, d.Author, d.Comment, 0)
					d.StartNotificationSent = true
				}
			}
			hst.ScheduledDowntimeDepth++
		}
	} else {
		svc := dm.store.GetService(d.HostName, d.ServiceDescription)
		if svc != nil {
			if svc.ScheduledDowntimeDepth == 0 {
				dm.log("SERVICE DOWNTIME ALERT: %s;%s;STARTED; %s on %s has entered a period of scheduled downtime",
					d.HostName, d.ServiceDescription, d.ServiceDescription, d.HostName)
				if !d.StartNotificationSent && dm.notifier != nil {
					dm.notifier.SendServiceNotification(d.HostName, d.ServiceDescription, objects.NotificationDowntimeStart, d.Author, d.Comment, 0)
					d.StartNotificationSent = true
				}
			}
			svc.ScheduledDowntimeDepth++
		}
	}

	// Start all triggered downtimes
	dm.mu.RLock()
	var triggered []uint64
	for tid, td := range dm.downtimes {
		if td.TriggeredBy == id && !td.IsInEffect {
			triggered = append(triggered, tid)
		}
	}
	dm.mu.RUnlock()
	for _, tid := range triggered {
		dm.HandleStart(tid)
	}
}

// HandleEnd processes a downtime end event.
func (dm *DowntimeManager) HandleEnd(id uint64) {
	dm.mu.RLock()
	d, ok := dm.downtimes[id]
	dm.mu.RUnlock()
	if !ok || !d.IsInEffect {
		return
	}

	dm.stopDowntime(d, false)

	// Delete comment
	if d.CommentID > 0 {
		dm.comments.Delete(d.CommentID)
	}

	// Stop triggered downtimes
	dm.mu.RLock()
	var triggered []uint64
	for tid, td := range dm.downtimes {
		if td.TriggeredBy == id && td.IsInEffect {
			triggered = append(triggered, tid)
		}
	}
	dm.mu.RUnlock()
	for _, tid := range triggered {
		dm.HandleEnd(tid)
	}

	dm.mu.Lock()
	delete(dm.downtimes, id)
	dm.mu.Unlock()
}

func (dm *DowntimeManager) stopDowntime(d *Downtime, cancelled bool) {
	d.IsInEffect = false
	action := "STOPPED"
	notifType := objects.NotificationDowntimeEnd
	if cancelled {
		action = "CANCELLED"
		notifType = objects.NotificationDowntimeCancelled
	}

	if d.Type == objects.HostDowntimeType {
		hst := dm.store.GetHost(d.HostName)
		if hst != nil {
			hst.ScheduledDowntimeDepth--
			if hst.ScheduledDowntimeDepth < 0 {
				hst.ScheduledDowntimeDepth = 0
			}
			if hst.ScheduledDowntimeDepth == 0 {
				dm.log("HOST DOWNTIME ALERT: %s;%s; %s has exited from a period of scheduled downtime",
					d.HostName, action, d.HostName)
				if dm.notifier != nil {
					dm.notifier.SendHostNotification(d.HostName, notifType, d.Author, d.Comment, 0)
				}
			}
		}
	} else {
		svc := dm.store.GetService(d.HostName, d.ServiceDescription)
		if svc != nil {
			svc.ScheduledDowntimeDepth--
			if svc.ScheduledDowntimeDepth < 0 {
				svc.ScheduledDowntimeDepth = 0
			}
			if svc.ScheduledDowntimeDepth == 0 {
				dm.log("SERVICE DOWNTIME ALERT: %s;%s;%s; %s on %s has exited from a period of scheduled downtime",
					d.HostName, d.ServiceDescription, action, d.ServiceDescription, d.HostName)
				if dm.notifier != nil {
					dm.notifier.SendServiceNotification(d.HostName, d.ServiceDescription, notifType, d.Author, d.Comment, 0)
				}
			}
		}
	}

	if d.IncrementedPendingDowntime {
		dm.decrementPending(d)
	}
}

// CheckPendingFlexHostDowntime checks if a flexible downtime should start for a host.
func (dm *DowntimeManager) CheckPendingFlexHostDowntime(hostName string, currentState int) {
	if currentState == objects.HostUp {
		return
	}
	now := time.Now()
	dm.mu.RLock()
	var toStart []uint64
	for _, d := range dm.downtimes {
		if d.Type != objects.HostDowntimeType || d.HostName != hostName {
			continue
		}
		if d.Fixed || d.IsInEffect || d.TriggeredBy != 0 {
			continue
		}
		if now.Before(d.StartTime) || now.After(d.EndTime) {
			continue
		}
		toStart = append(toStart, d.DowntimeID)
	}
	dm.mu.RUnlock()

	for _, id := range toStart {
		dm.mu.RLock()
		d := dm.downtimes[id]
		dm.mu.RUnlock()
		if d != nil {
			d.FlexDowntimeStart = now
			dm.HandleStart(id)
		}
	}
}

// CheckPendingFlexServiceDowntime checks if a flexible downtime should start for a service.
func (dm *DowntimeManager) CheckPendingFlexServiceDowntime(hostName, svcDesc string, currentState int) {
	if currentState == objects.ServiceOK {
		return
	}
	now := time.Now()
	dm.mu.RLock()
	var toStart []uint64
	for _, d := range dm.downtimes {
		if d.Type != objects.ServiceDowntimeType || d.HostName != hostName || d.ServiceDescription != svcDesc {
			continue
		}
		if d.Fixed || d.IsInEffect || d.TriggeredBy != 0 {
			continue
		}
		if now.Before(d.StartTime) || now.After(d.EndTime) {
			continue
		}
		toStart = append(toStart, d.DowntimeID)
	}
	dm.mu.RUnlock()

	for _, id := range toStart {
		dm.mu.RLock()
		d := dm.downtimes[id]
		dm.mu.RUnlock()
		if d != nil {
			d.FlexDowntimeStart = now
			dm.HandleStart(id)
		}
	}
}

// CheckExpired removes expired downtimes that never triggered.
func (dm *DowntimeManager) CheckExpired() {
	now := time.Now()
	dm.mu.RLock()
	var expired []uint64
	for id, d := range dm.downtimes {
		if !d.IsInEffect && !d.EndTime.IsZero() && d.EndTime.Before(now) {
			expired = append(expired, id)
		}
	}
	dm.mu.RUnlock()

	for _, id := range expired {
		dm.mu.RLock()
		d := dm.downtimes[id]
		dm.mu.RUnlock()
		if d == nil {
			continue
		}
		// Send end notification for expired flex downtimes
		if dm.notifier != nil {
			if d.Type == objects.HostDowntimeType {
				dm.notifier.SendHostNotification(d.HostName, objects.NotificationDowntimeEnd, d.Author, d.Comment, 0)
			} else {
				dm.notifier.SendServiceNotification(d.HostName, d.ServiceDescription, objects.NotificationDowntimeEnd, d.Author, d.Comment, 0)
			}
		}
		if d.CommentID > 0 {
			dm.comments.Delete(d.CommentID)
		}
		if d.IncrementedPendingDowntime {
			dm.decrementPending(d)
		}
		dm.mu.Lock()
		delete(dm.downtimes, id)
		dm.mu.Unlock()
	}
}

// FlexEndTime returns the actual end time for a flexible downtime.
func (d *Downtime) FlexEndTime() time.Time {
	if !d.Fixed && !d.FlexDowntimeStart.IsZero() {
		return d.FlexDowntimeStart.Add(d.Duration)
	}
	return d.EndTime
}

func (dm *DowntimeManager) incrementPending(d *Downtime) {
	if d.IncrementedPendingDowntime {
		return
	}
	d.IncrementedPendingDowntime = true
	if d.Type == objects.HostDowntimeType {
		if hst := dm.store.GetHost(d.HostName); hst != nil {
			hst.PendingFlexDowntime++
		}
	} else {
		if svc := dm.store.GetService(d.HostName, d.ServiceDescription); svc != nil {
			svc.PendingFlexDowntime++
		}
	}
}

func (dm *DowntimeManager) decrementPending(d *Downtime) {
	if !d.IncrementedPendingDowntime {
		return
	}
	d.IncrementedPendingDowntime = false
	if d.Type == objects.HostDowntimeType {
		if hst := dm.store.GetHost(d.HostName); hst != nil {
			hst.PendingFlexDowntime--
		}
	} else {
		if svc := dm.store.GetService(d.HostName, d.ServiceDescription); svc != nil {
			svc.PendingFlexDowntime--
		}
	}
}

// Get returns a downtime by ID.
func (dm *DowntimeManager) Get(id uint64) *Downtime {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.downtimes[id]
}

// All returns all downtimes sorted by start time.
func (dm *DowntimeManager) All() []*Downtime {
	dm.mu.RLock()
	result := make([]*Downtime, 0, len(dm.downtimes))
	for _, d := range dm.downtimes {
		result = append(result, d)
	}
	dm.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool {
		if result[i].StartTime.Equal(result[j].StartTime) {
			// Untriggered sorts before triggered
			return result[i].TriggeredBy == 0 && result[j].TriggeredBy != 0
		}
		return result[i].StartTime.Before(result[j].StartTime)
	})
	return result
}

// NextID returns the next downtime ID value.
func (dm *DowntimeManager) NextID() uint64 {
	return dm.nextID.Load()
}

// DeleteByHost removes all downtimes for a host.
func (dm *DowntimeManager) DeleteByHost(hostName string) {
	dm.mu.RLock()
	var ids []uint64
	for id, d := range dm.downtimes {
		if d.HostName == hostName {
			ids = append(ids, id)
		}
	}
	dm.mu.RUnlock()
	for _, id := range ids {
		dm.Unschedule(id)
	}
}

func downtimeTypeName(t int) string {
	if t == objects.HostDowntimeType {
		return "host"
	}
	return "service"
}
