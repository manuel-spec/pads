package model

import "time"

const StateVersion = 1

// SegmentStatus describes segment lifecycle state.
type SegmentStatus string

const (
	SegmentPending  SegmentStatus = "pending"
	SegmentActive   SegmentStatus = "active"
	SegmentComplete SegmentStatus = "complete"
	SegmentFailed   SegmentStatus = "failed"
)

// ConnectionState describes connection lifecycle state.
type ConnectionState string

const (
	ConnectionOpen     ConnectionState = "open"
	ConnectionWarming  ConnectionState = "warming"
	ConnectionActive   ConnectionState = "active"
	ConnectionDraining ConnectionState = "draining"
	ConnectionClosed   ConnectionState = "closed"
)

// Segment represents a byte range assigned to one worker.
type Segment struct {
	ID              string        `json:"id"`
	ByteStart       int64         `json:"byte_start"`
	ByteEnd         int64         `json:"byte_end"`
	BytesDownloaded int64         `json:"bytes_downloaded"`
	Status          SegmentStatus `json:"status"`
	AssignedConn    string        `json:"assigned_conn,omitempty"`
	TempPath        string        `json:"temp_path,omitempty"`
	StartedAt       time.Time     `json:"started_at,omitempty"`
	UpdatedAt       time.Time     `json:"updated_at,omitempty"`
	SpeedBPS        float64       `json:"speed_bps,omitempty"`
	ETASeconds      float64       `json:"eta_seconds,omitempty"`
}

// ByteLength returns the inclusive byte length of the segment.
func (s Segment) ByteLength() int64 {
	return s.ByteEnd - s.ByteStart + 1
}

// RemainingBytes returns bytes left to download in this segment.
func (s Segment) RemainingBytes() int64 {
	remaining := s.ByteLength() - s.BytesDownloaded
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Connection represents one active HTTP worker.
type Connection struct {
	ID              string          `json:"id"`
	State           ConnectionState `json:"state"`
	CurrentSegment  string          `json:"current_segment,omitempty"`
	WarmupStartedAt time.Time       `json:"warmup_started_at,omitempty"`
	CurrentSpeedBPS float64         `json:"current_speed_bps"`
	PeakSpeedBPS    float64         `json:"peak_speed_bps"`
	OpenedAt        time.Time       `json:"opened_at"`
}

// ServerProfile captures probe results used for scheduling hints.
type ServerProfile struct {
	RangeSupported       bool    `json:"range_supported"`
	ContentLength        int64   `json:"content_length"`
	ContentType          string  `json:"content_type"`
	HTTPVersion          string  `json:"http_version"`
	LatencyMS            float64 `json:"latency_ms"`
	ThrottleHint         string  `json:"throttle_hint,omitempty"`
	RecommendedConns     int     `json:"recommended_connections"`
	AcceptRanges         string  `json:"accept_ranges,omitempty"`
	ContentLengthTrusted bool    `json:"content_length_trusted"`
}

// DownloadState is persisted resume state for a download.
type DownloadState struct {
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Output    string    `json:"output"`
	TotalSize int64     `json:"total_size"`
	Segments  []Segment `json:"segments"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Complete  bool      `json:"complete"`
}

// SchedulerPhase describes the current scheduler phase.
type SchedulerPhase string

const (
	PhaseProbe  SchedulerPhase = "probe"
	PhaseRamp   SchedulerPhase = "ramp"
	PhaseCruise SchedulerPhase = "cruise"
	PhaseTail   SchedulerPhase = "tail"
	PhaseDone   SchedulerPhase = "done"
)
