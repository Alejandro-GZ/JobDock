package domain

import (
	"encoding/json"
	"time"
)

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type JobStatus string

const (
	JobQueued       JobStatus = "QUEUED"
	JobAssigned     JobStatus = "ASSIGNED"
	JobPullingImage JobStatus = "PULLING_IMAGE"
	JobStarting     JobStatus = "STARTING"
	JobRunning      JobStatus = "RUNNING"
	JobStopping     JobStatus = "STOPPING"
	JobCancelled    JobStatus = "CANCELLED"
	JobSucceeded    JobStatus = "SUCCEEDED"
	JobFailed       JobStatus = "FAILED"
	JobLost         JobStatus = "LOST"
	JobDeleting     JobStatus = "DELETING"
	JobDeleted      JobStatus = "DELETED"
)

type NodeStatus string

const (
	NodeOnline   NodeStatus = "ONLINE"
	NodeDraining NodeStatus = "DRAINING"
	NodeOffline  NodeStatus = "OFFLINE"
	NodeDegraded NodeStatus = "DEGRADED"
)

type BuildStatus string

const (
	BuildCreated   BuildStatus = "CREATED"
	BuildAnalyzing BuildStatus = "ANALYZING"
	BuildBuilding  BuildStatus = "BUILDING"
	BuildSucceeded BuildStatus = "SUCCEEDED"
	BuildFailed    BuildStatus = "FAILED"
	BuildCancelled BuildStatus = "CANCELLED"
)

type BuildMode string

const (
	BuildModeRailpack   BuildMode = "RAILPACK"
	BuildModeDockerfile BuildMode = "DOCKERFILE"
)

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type BuildSource struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

type Build struct {
	ID                string      `json:"id"`
	OwnerID           string      `json:"owner_id"`
	Name              string      `json:"name"`
	Mode              BuildMode   `json:"mode"`
	Status            BuildStatus `json:"status"`
	Source            BuildSource `json:"source"`
	ContextPath       string      `json:"context_path,omitempty"`
	DockerfilePath    string      `json:"dockerfile_path,omitempty"`
	OCIDigest         string      `json:"oci_digest,omitempty"`
	ArtifactReference string      `json:"artifact_reference,omitempty"`
	ArtifactAvailable bool        `json:"artifact_available"`
	FailureReason     string      `json:"failure_reason,omitempty"`
	CreatedAt         time.Time   `json:"created_at"`
	StartedAt         *time.Time  `json:"started_at,omitempty"`
	FinishedAt        *time.Time  `json:"finished_at,omitempty"`
	Version           int64       `json:"version"`
}

type ManagedArtifact struct {
	BuildID          string    `json:"build_id"`
	OwnerID          string    `json:"owner_id,omitempty"`
	Digest           string    `json:"digest"`
	SHA256           string    `json:"sha256"`
	Size             int64     `json:"size"`
	MediaType        string    `json:"media_type"`
	RuntimeImage     string    `json:"runtime_image,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	LastReferencedAt time.Time `json:"last_referenced_at"`
}

type BuildEvent struct {
	ID        int64       `json:"id"`
	BuildID   string      `json:"build_id"`
	Status    BuildStatus `json:"status"`
	Message   string      `json:"message,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

type BuildPlan struct {
	BuildID         string          `json:"build_id"`
	Provider        string          `json:"provider"`
	Runtime         string          `json:"runtime"`
	PackageManager  string          `json:"package_manager"`
	Entrypoint      string          `json:"entrypoint"`
	RailpackVersion string          `json:"railpack_version"`
	Plan            json.RawMessage `json:"plan"`
	Info            json.RawMessage `json:"info"`
	CreatedAt       time.Time       `json:"created_at"`
	ConfirmedAt     *time.Time      `json:"confirmed_at,omitempty"`
}

type BuildAssignmentStatus string

const (
	BuildAssignmentPending   BuildAssignmentStatus = "PENDING"
	BuildAssignmentRunning   BuildAssignmentStatus = "RUNNING"
	BuildAssignmentSucceeded BuildAssignmentStatus = "SUCCEEDED"
	BuildAssignmentFailed    BuildAssignmentStatus = "FAILED"
	BuildAssignmentCancelled BuildAssignmentStatus = "CANCELLED"
)

type BuildAssignment struct {
	ID              string                `json:"id"`
	BuildID         string                `json:"build_id"`
	Status          BuildAssignmentStatus `json:"status"`
	CancelRequested bool                  `json:"cancel_requested"`
	BuilderID       string                `json:"builder_id,omitempty"`
	LeaseExpiresAt  *time.Time            `json:"lease_expires_at,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
}

type BuildWork struct {
	Assignment BuildAssignment `json:"assignment"`
	Build      Build           `json:"build"`
	Plan       *BuildPlan      `json:"plan,omitempty"`
}

type GPURequest struct {
	Count        int   `json:"count"`
	MinVRAMBytes int64 `json:"min_vram_bytes"`
}

type Resources struct {
	CPUMillis   int64      `json:"cpu_millis"`
	MemoryBytes int64      `json:"memory_bytes"`
	GPU         GPURequest `json:"gpu"`
}

type SecretRef struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Mode   string `json:"mode"`
}

type InputFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type JobSpec struct {
	Name             string            `json:"name"`
	Image            string            `json:"image"`
	Command          []string          `json:"command"`
	WorkingDirectory string            `json:"working_directory,omitempty"`
	Environment      map[string]string `json:"environment,omitempty"`
	SecretRefs       []SecretRef       `json:"secret_refs,omitempty"`
	RegistrySecret   string            `json:"registry_secret,omitempty"`
	Resources        Resources         `json:"resources"`
	Labels           map[string]string `json:"labels,omitempty"`
	NodeSelector     map[string]string `json:"node_selector,omitempty"`
	Inputs           []InputFile       `json:"inputs,omitempty"`
}

type Job struct {
	ID              string     `json:"id"`
	OwnerID         string     `json:"owner_id"`
	Spec            JobSpec    `json:"spec"`
	Status          JobStatus  `json:"status"`
	DesiredStatus   JobStatus  `json:"desired_status"`
	ObservedStatus  JobStatus  `json:"observed_status"`
	AssignedNodeID  string     `json:"assigned_node_id,omitempty"`
	AttemptID       string     `json:"attempt_id,omitempty"`
	ImageDigest     string     `json:"image_digest,omitempty"`
	ExitCode        *int       `json:"exit_code,omitempty"`
	QueueReasonCode string     `json:"queue_reason_code,omitempty"`
	QueueReason     string     `json:"queue_reason,omitempty"`
	FailureReason   string     `json:"failure_reason,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
	Version         int64      `json:"version"`
}

type OutputFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type JobAttempt struct {
	ID            string       `json:"id"`
	JobID         string       `json:"job_id"`
	AttemptNumber int          `json:"attempt_number"`
	NodeID        string       `json:"node_id"`
	Status        JobStatus    `json:"status"`
	ImageDigest   string       `json:"image_digest,omitempty"`
	ExitCode      *int         `json:"exit_code,omitempty"`
	FailureReason string       `json:"failure_reason,omitempty"`
	Outputs       []OutputFile `json:"outputs"`
	CreatedAt     time.Time    `json:"created_at"`
	StartedAt     *time.Time   `json:"started_at,omitempty"`
	FinishedAt    *time.Time   `json:"finished_at,omitempty"`
}

type GPU struct {
	UUID      string `json:"uuid"`
	Model     string `json:"model"`
	VRAMBytes int64  `json:"vram_bytes"`
	Allocated bool   `json:"allocated"`
}

type GPUDiscovery struct {
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
	Message   string `json:"message,omitempty"`
}

type Node struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Status               NodeStatus        `json:"status"`
	AgentVersion         string            `json:"agent_version"`
	ProtocolVersion      int               `json:"protocol_version"`
	Architecture         string            `json:"architecture"`
	DockerVersion        string            `json:"docker_version"`
	CPUTotalMillis       int64             `json:"cpu_total_millis"`
	CPUAllocatedMillis   int64             `json:"cpu_allocated_millis"`
	MemoryTotalBytes     int64             `json:"memory_total_bytes"`
	MemoryAllocatedBytes int64             `json:"memory_allocated_bytes"`
	WorkspaceFreeBytes   int64             `json:"workspace_free_bytes"`
	Labels               map[string]string `json:"labels"`
	GPUs                 []GPU             `json:"gpus"`
	GPUDiscovery         GPUDiscovery      `json:"gpu_discovery"`
	LastHeartbeat        time.Time         `json:"last_heartbeat"`
	CreatedAt            time.Time         `json:"created_at"`
}

type Assignment struct {
	ID                string            `json:"id"`
	JobID             string            `json:"job_id"`
	AttemptID         string            `json:"attempt_id"`
	Spec              JobSpec           `json:"spec"`
	GPUUUIDs          []string          `json:"gpu_uuids"`
	JobToken          string            `json:"job_token"`
	JobTokenEncrypted []byte            `json:"-"`
	Secrets           map[string]string `json:"secrets,omitempty"`
	RegistryAuth      string            `json:"registry_auth,omitempty"`
	ManagedArtifact   *ManagedArtifact  `json:"managed_artifact,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	EventSequence     int64             `json:"event_sequence"`
}

type CheckpointSync struct {
	ID          string     `json:"id"`
	JobID       string     `json:"job_id"`
	AttemptID   string     `json:"attempt_id"`
	Status      string     `json:"status"`
	FileCount   int        `json:"file_count"`
	ByteCount   int64      `json:"byte_count"`
	RequestedAt time.Time  `json:"requested_at"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}

type CheckpointFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type Event struct {
	ID        int64          `json:"id"`
	JobID     string         `json:"job_id"`
	AttemptID string         `json:"attempt_id,omitempty"`
	Sequence  int64          `json:"sequence"`
	Type      string         `json:"type"`
	Status    JobStatus      `json:"status,omitempty"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

// ResourceSample is deliberately limited to normalized scalar resource data.
// Docker Stats responses must never cross this boundary or reach persistence.
type ResourceSample struct {
	Cursor                    int64     `json:"cursor,omitempty"`
	JobID                     string    `json:"-"`
	AttemptID                 string    `json:"attempt_id"`
	CapturedAt                time.Time `json:"captured_at"`
	ResolutionSeconds         int       `json:"resolution_seconds"`
	SampleCount               int       `json:"sample_count"`
	CPUMillis                 int64     `json:"cpu_millis"`
	MemoryBytes               int64     `json:"memory_bytes"`
	GPUUtilizationBasisPoints *int64    `json:"gpu_utilization_basis_points,omitempty"`
	GPUMemoryBytes            *int64    `json:"gpu_memory_bytes,omitempty"`
}

type MetricSample struct {
	Cursor     int64          `json:"cursor,omitempty"`
	JobID      string         `json:"-"`
	AttemptID  string         `json:"attempt_id"`
	Name       string         `json:"name"`
	Step       *int64         `json:"step,omitempty"`
	Value      float64        `json:"value"`
	CapturedAt time.Time      `json:"captured_at"`
	Unit       string         `json:"unit,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type MetricPoint struct {
	Cursor      int64     `json:"cursor,omitempty"`
	CapturedAt  time.Time `json:"captured_at"`
	Step        *int64    `json:"step,omitempty"`
	Value       float64   `json:"value"`
	SampleCount int64     `json:"sample_count"`
}

type MetricSeries struct {
	Name        string         `json:"name"`
	Unit        string         `json:"unit,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Points      []MetricPoint  `json:"points"`
	Last        float64        `json:"last"`
	Min         float64        `json:"min"`
	Max         float64        `json:"max"`
	SampleCount int64          `json:"sample_count"`
}

// ObservationContext is the presentation-agnostic context shared by richer
// observability primitives. Specialized persistence is introduced by the
// stories that make each primitive queryable.
type ObservationContext struct {
	Step       *int64         `json:"step,omitempty"`
	CapturedAt *time.Time     `json:"timestamp,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type CheckpointObservation struct {
	Label string `json:"label,omitempty"`
	ObservationContext
}

type Milestone struct {
	Name     string         `json:"name"`
	Weight   *float64       `json:"weight,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ProgressObservation struct {
	Value     float64 `json:"value"`
	Milestone string  `json:"milestone,omitempty"`
	ObservationContext
}

type MatrixObservation struct {
	Name   string      `json:"name"`
	Values [][]float64 `json:"values"`
	Labels []string    `json:"labels,omitempty"`
	ObservationContext
}

func IsActive(status JobStatus) bool {
	switch status {
	case JobAssigned, JobPullingImage, JobStarting, JobRunning, JobStopping, JobLost:
		return true
	default:
		return false
	}
}

func CanTransition(from, to JobStatus) bool {
	if from == to {
		return true
	}
	allowed := map[JobStatus]map[JobStatus]bool{
		JobQueued:       {JobAssigned: true, JobCancelled: true, JobDeleting: true},
		JobAssigned:     {JobPullingImage: true, JobStarting: true, JobRunning: true, JobStopping: true, JobFailed: true, JobLost: true},
		JobPullingImage: {JobStarting: true, JobStopping: true, JobFailed: true, JobLost: true},
		JobStarting:     {JobRunning: true, JobStopping: true, JobFailed: true, JobLost: true},
		JobRunning:      {JobStopping: true, JobSucceeded: true, JobFailed: true, JobLost: true},
		JobStopping:     {JobCancelled: true, JobFailed: true, JobLost: true},
		JobLost:         {JobRunning: true, JobStopping: true, JobSucceeded: true, JobFailed: true, JobCancelled: true},
		JobCancelled:    {JobDeleting: true},
		JobSucceeded:    {JobDeleting: true},
		JobFailed:       {JobDeleting: true},
		JobDeleting:     {JobDeleted: true},
	}
	return allowed[from][to]
}

func CanBuildTransition(from, to BuildStatus) bool {
	if from == to {
		return true
	}
	allowed := map[BuildStatus]map[BuildStatus]bool{
		BuildCreated:   {BuildAnalyzing: true, BuildBuilding: true, BuildFailed: true, BuildCancelled: true},
		BuildAnalyzing: {BuildBuilding: true, BuildFailed: true, BuildCancelled: true},
		BuildBuilding:  {BuildSucceeded: true, BuildFailed: true, BuildCancelled: true},
	}
	return allowed[from][to]
}
