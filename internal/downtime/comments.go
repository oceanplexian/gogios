// Package downtime implements scheduled downtime and comment management.
package downtime

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// Comment represents a Nagios comment (host or service).
type Comment struct {
	CommentType        int    // HostCommentType or ServiceCommentType
	EntryType          int    // UserCommentEntry, DowntimeCommentEntry, etc.
	CommentID          uint64
	Source             int    // 0=internal, 1=external
	Persistent         bool
	EntryTime          time.Time
	Expires            bool
	ExpireTime         time.Time
	HostName           string
	ServiceDescription string
	Author             string
	Data               string
}

// CommentManager manages all comments.
type CommentManager struct {
	mu       sync.RWMutex
	comments map[uint64]*Comment
	nextID   atomic.Uint64
}

// NewCommentManager creates a new comment manager.
func NewCommentManager(startID uint64) *CommentManager {
	cm := &CommentManager{
		comments: make(map[uint64]*Comment),
	}
	cm.nextID.Store(startID)
	return cm
}

// Add adds a comment and returns its ID.
func (cm *CommentManager) Add(c *Comment) uint64 {
	id := cm.nextID.Add(1) - 1
	c.CommentID = id
	if c.EntryTime.IsZero() {
		c.EntryTime = time.Now()
	}
	cm.mu.Lock()
	cm.comments[id] = c
	cm.mu.Unlock()
	return id
}

// AddWithID adds a comment with a specific ID (for retention restore).
func (cm *CommentManager) AddWithID(c *Comment) {
	cm.mu.Lock()
	cm.comments[c.CommentID] = c
	cm.mu.Unlock()
	// Ensure nextID stays ahead
	for {
		cur := cm.nextID.Load()
		if c.CommentID < cur {
			break
		}
		if cm.nextID.CompareAndSwap(cur, c.CommentID+1) {
			break
		}
	}
}

// Delete removes a comment by ID.
func (cm *CommentManager) Delete(id uint64) {
	cm.mu.Lock()
	delete(cm.comments, id)
	cm.mu.Unlock()
}

// Get returns a comment by ID.
func (cm *CommentManager) Get(id uint64) *Comment {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.comments[id]
}

// DeleteAllForHost deletes all comments for a host.
func (cm *CommentManager) DeleteAllForHost(hostName string) {
	cm.mu.Lock()
	for id, c := range cm.comments {
		if c.HostName == hostName && c.CommentType == objects.HostCommentType {
			delete(cm.comments, id)
		}
	}
	cm.mu.Unlock()
}

// DeleteAllForService deletes all comments for a specific service.
func (cm *CommentManager) DeleteAllForService(hostName, svcDesc string) {
	cm.mu.Lock()
	for id, c := range cm.comments {
		if c.HostName == hostName && c.ServiceDescription == svcDesc && c.CommentType == objects.ServiceCommentType {
			delete(cm.comments, id)
		}
	}
	cm.mu.Unlock()
}

// DeleteAckComments deletes non-persistent acknowledgement comments for a host.
func (cm *CommentManager) DeleteHostAckComments(hostName string) {
	cm.mu.Lock()
	for id, c := range cm.comments {
		if c.HostName == hostName && c.CommentType == objects.HostCommentType &&
			c.EntryType == objects.AcknowledgementCommentEntry && !c.Persistent {
			delete(cm.comments, id)
		}
	}
	cm.mu.Unlock()
}

// DeleteServiceAckComments deletes non-persistent acknowledgement comments for a service.
func (cm *CommentManager) DeleteServiceAckComments(hostName, svcDesc string) {
	cm.mu.Lock()
	for id, c := range cm.comments {
		if c.HostName == hostName && c.ServiceDescription == svcDesc &&
			c.CommentType == objects.ServiceCommentType &&
			c.EntryType == objects.AcknowledgementCommentEntry && !c.Persistent {
			delete(cm.comments, id)
		}
	}
	cm.mu.Unlock()
}

// ExpireComments removes expired comments.
func (cm *CommentManager) ExpireComments() {
	now := time.Now()
	cm.mu.Lock()
	for id, c := range cm.comments {
		if c.Expires && !c.ExpireTime.IsZero() && c.ExpireTime.Before(now) {
			delete(cm.comments, id)
		}
	}
	cm.mu.Unlock()
}

// All returns all comments.
func (cm *CommentManager) All() []*Comment {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]*Comment, 0, len(cm.comments))
	for _, c := range cm.comments {
		result = append(result, c)
	}
	return result
}

// ForHost returns all comments for a host.
func (cm *CommentManager) ForHost(hostName string) []*Comment {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	var result []*Comment
	for _, c := range cm.comments {
		if c.HostName == hostName && c.CommentType == objects.HostCommentType {
			result = append(result, c)
		}
	}
	return result
}

// ForService returns all comments for a service.
func (cm *CommentManager) ForService(hostName, svcDesc string) []*Comment {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	var result []*Comment
	for _, c := range cm.comments {
		if c.HostName == hostName && c.ServiceDescription == svcDesc && c.CommentType == objects.ServiceCommentType {
			result = append(result, c)
		}
	}
	return result
}

// NextID returns the next comment ID value.
func (cm *CommentManager) NextID() uint64 {
	return cm.nextID.Load()
}
