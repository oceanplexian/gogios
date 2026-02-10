package downtime

import (
	"testing"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

func TestCommentManager_AddAndGet(t *testing.T) {
	cm := NewCommentManager(1)
	c := &Comment{
		CommentType: objects.HostCommentType,
		EntryType:   objects.UserCommentEntry,
		HostName:    "host1",
		Author:      "admin",
		Data:        "Test comment",
		Persistent:  true,
	}
	id := cm.Add(c)
	if id == 0 {
		t.Error("expected non-zero comment ID")
	}
	got := cm.Get(id)
	if got == nil {
		t.Fatal("expected to find comment")
	}
	if got.Data != "Test comment" {
		t.Errorf("expected 'Test comment', got '%s'", got.Data)
	}
}

func TestCommentManager_Delete(t *testing.T) {
	cm := NewCommentManager(1)
	id := cm.Add(&Comment{HostName: "host1", CommentType: objects.HostCommentType})
	cm.Delete(id)
	if cm.Get(id) != nil {
		t.Error("expected comment to be deleted")
	}
}

func TestCommentManager_DeleteAllForHost(t *testing.T) {
	cm := NewCommentManager(1)
	cm.Add(&Comment{HostName: "host1", CommentType: objects.HostCommentType})
	cm.Add(&Comment{HostName: "host1", CommentType: objects.HostCommentType})
	cm.Add(&Comment{HostName: "host2", CommentType: objects.HostCommentType})
	cm.DeleteAllForHost("host1")
	if len(cm.ForHost("host1")) != 0 {
		t.Error("expected all host1 comments deleted")
	}
	if len(cm.ForHost("host2")) != 1 {
		t.Error("expected host2 comment to remain")
	}
}

func TestCommentManager_DeleteAckComments(t *testing.T) {
	cm := NewCommentManager(1)
	cm.Add(&Comment{
		HostName:    "host1",
		CommentType: objects.HostCommentType,
		EntryType:   objects.AcknowledgementCommentEntry,
		Persistent:  false,
	})
	cm.Add(&Comment{
		HostName:    "host1",
		CommentType: objects.HostCommentType,
		EntryType:   objects.AcknowledgementCommentEntry,
		Persistent:  true,
	})
	cm.Add(&Comment{
		HostName:    "host1",
		CommentType: objects.HostCommentType,
		EntryType:   objects.UserCommentEntry,
	})
	cm.DeleteHostAckComments("host1")
	comments := cm.ForHost("host1")
	if len(comments) != 2 {
		t.Errorf("expected 2 remaining comments (persistent ack + user), got %d", len(comments))
	}
}

func TestCommentManager_ExpireComments(t *testing.T) {
	cm := NewCommentManager(1)
	cm.Add(&Comment{
		HostName:    "host1",
		CommentType: objects.HostCommentType,
		Expires:     true,
		ExpireTime:  time.Now().Add(-time.Hour),
	})
	cm.Add(&Comment{
		HostName:    "host1",
		CommentType: objects.HostCommentType,
		Expires:     false,
	})
	cm.ExpireComments()
	if len(cm.All()) != 1 {
		t.Errorf("expected 1 comment after expiry, got %d", len(cm.All()))
	}
}

func TestCommentManager_ForService(t *testing.T) {
	cm := NewCommentManager(1)
	cm.Add(&Comment{
		HostName:           "host1",
		ServiceDescription: "HTTP",
		CommentType:        objects.ServiceCommentType,
	})
	cm.Add(&Comment{
		HostName:           "host1",
		ServiceDescription: "SSH",
		CommentType:        objects.ServiceCommentType,
	})
	svcComments := cm.ForService("host1", "HTTP")
	if len(svcComments) != 1 {
		t.Errorf("expected 1 HTTP comment, got %d", len(svcComments))
	}
}
