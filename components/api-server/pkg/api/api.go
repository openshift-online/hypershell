package api

// Import core TRex API types
import (
	trexapi "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
)

// Re-export TRex types for convenience
type Meta = trexapi.Meta
type EventType = trexapi.EventType

// Re-export TRex constants
const (
	CreateEventType = trexapi.CreateEventType
	UpdateEventType = trexapi.UpdateEventType
	DeleteEventType = trexapi.DeleteEventType
)

// Re-export TRex functions
var NewID = trexapi.NewID
var (
	Version   string
	BuildTime string
)
