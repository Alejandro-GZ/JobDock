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
	Count        int      `json:"count"`
	MinVRAMBytes int64    `json:"min_vram_bytes"`
	UUIDs        []string `json:"uuids,omitempty"`
}

type Resources struct {
	CPUMillis    int64      `json:"cpu_millis"`
	CPUPackageID string     `json:"cpu_package_id,omitempty"`
	MemoryBytes  int64      `json:"memory_bytes"`
	GPU          GPURequest `json:"gpu"`
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
	TargetNodeID     string            `json:"target_node_id,omitempty"`
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
	CPUPackageID  string       `json:"cpu_package_id,omitempty"`
	GPUUUIDs      []string     `json:"gpu_uuids,omitempty"`
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
	UUID                   string `json:"uuid"`
	Model                  string `json:"model"`
	VRAMBytes              int64  `json:"vram_bytes"`
	PCIBusID               string `json:"pci_bus_id,omitempty"`
	DriverVersion          string `json:"driver_version,omitempty"`
	ComputeCapability      string `json:"compute_capability,omitempty"`
	UtilizationBasisPoints *int64 `json:"utilization_basis_points,omitempty"`
	MemoryUsedBytes        *int64 `json:"memory_used_bytes,omitempty"`
	TemperatureCelsius     *int64 `json:"temperature_celsius,omitempty"`
	Allocated              bool   `json:"allocated"`
	AllocatedJobID         string `json:"allocated_job_id,omitempty"`
}

type CPUPackage struct {
	ID              string `json:"id"`
	Vendor          string `json:"vendor,omitempty"`
	Model           string `json:"model"`
	PhysicalCores   int    `json:"physical_cores"`
	LogicalCPUs     []int  `json:"logical_cpus"`
	TotalMillis     int64  `json:"total_millis"`
	AllocatedMillis int64  `json:"allocated_millis"`
}

type NodeSystemInfo struct {
	Hostname        string `json:"hostname,omitempty"`
	OperatingSystem string `json:"operating_system,omitempty"`
	OSVersion       string `json:"os_version,omitempty"`
	OSType          string `json:"os_type,omitempty"`
	KernelVersion   string `json:"kernel_version,omitempty"`
	Architecture    string `json:"architecture,omitempty"`
}

type NodeRuntimeInfo struct {
	DockerVersion string `json:"docker_version,omitempty"`
	StorageDriver string `json:"storage_driver,omitempty"`
	CgroupDriver  string `json:"cgroup_driver,omitempty"`
	CgroupVersion string `json:"cgroup_version,omitempty"`
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
	WorkspaceTotalBytes  int64             `json:"workspace_total_bytes"`
	WorkspaceFreeBytes   int64             `json:"workspace_free_bytes"`
	Labels               map[string]string `json:"labels"`
	Capabilities         []string          `json:"capabilities"`
	CPUPackages          []CPUPackage      `json:"cpu_packages"`
	GPUs                 []GPU             `json:"gpus"`
	GPUDiscovery         GPUDiscovery      `json:"gpu_discovery"`
	System               NodeSystemInfo    `json:"system"`
	Runtime              NodeRuntimeInfo   `json:"runtime"`
	LastHeartbeat        time.Time         `json:"last_heartbeat"`
	CreatedAt            time.Time         `json:"created_at"`
}

type Assignment struct {
	ID                string            `json:"id"`
	JobID             string            `json:"job_id"`
	AttemptID         string            `json:"attempt_id"`
	Spec              JobSpec           `json:"spec"`
	GPUUUIDs          []string          `json:"gpu_uuids"`
	CPUPackageID      string            `json:"cpu_package_id,omitempty"`
	CPUSet            string            `json:"cpu_set,omitempty"`
	JobToken          string            `json:"job_token"`
	JobTokenEncrypted []byte            `json:"-"`
	Secrets           map[string]string `json:"secrets,omitempty"`
	RegistryAuth      string            `json:"registry_auth,omitempty"`
	ManagedArtifact   *ManagedArtifact  `json:"managed_artifact,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	EventSequence     int64             `json:"event_sequence"`
}

type CheckpointSync struct {
	Cursor      int64          `json:"cursor,omitempty"`
	ID          string         `json:"id"`
	JobID       string         `json:"job_id"`
	AttemptID   string         `json:"attempt_id"`
	Status      string         `json:"status"`
	FileCount   int            `json:"file_count"`
	ByteCount   int64          `json:"byte_count"`
	RequestedAt time.Time      `json:"requested_at"`
	ConfirmedAt *time.Time     `json:"confirmed_at,omitempty"`
	Label       string         `json:"label,omitempty"`
	Step        *int64         `json:"step,omitempty"`
	ObservedAt  *time.Time     `json:"timestamp,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
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
	Tags       []string       `json:"tags,omitempty"`
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
	Tags        []string       `json:"tags,omitempty"`
	Points      []MetricPoint  `json:"points"`
	Last        float64        `json:"last"`
	Min         float64        `json:"min"`
	Max         float64        `json:"max"`
	Avg         float64        `json:"avg"`
	SampleCount int64          `json:"sample_count"`
}

// ObservableSourceDeclaration describes an attempt-scoped source before it
// emits data. Phase and milestone are structural scopes and deliberately do
// not reuse the numeric observation step.
type ObservableSourceDeclaration struct {
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Unit      string         `json:"unit,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Phase     string         `json:"phase,omitempty"`
	Milestone string         `json:"milestone,omitempty"`
}

type ObservabilityPhaseDeclaration struct {
	ID       string         `json:"id"`
	Name     string         `json:"name,omitempty"`
	Order    *int           `json:"order,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
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
	ID           int64             `json:"id,omitempty"`
	JobID        string            `json:"-"`
	AttemptID    string            `json:"attempt_id,omitempty"`
	Name         string            `json:"name"`
	MatrixType   string            `json:"matrix_type,omitempty"`
	Values       [][]*float64      `json:"values"`
	Labels       []string          `json:"labels,omitempty"`
	RowLabels    []string          `json:"row_labels,omitempty"`
	ColumnLabels []string          `json:"column_labels,omitempty"`
	Unit         string            `json:"unit,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	Resolution   *MatrixResolution `json:"resolution,omitempty"`
	ObservationContext
}

type MatrixResolution struct {
	Mode            string `json:"mode"`
	OriginalRows    int    `json:"original_rows"`
	OriginalColumns int    `json:"original_columns"`
	Rows            int    `json:"rows"`
	Columns         int    `json:"columns"`
}

// DistributionObservation is a bounded statistical snapshot. Group identifies
// a class, label, feature population, or comparison cohort such as baseline.
type DistributionObservation struct {
	ID        int64              `json:"id,omitempty"`
	JobID     string             `json:"-"`
	AttemptID string             `json:"attempt_id,omitempty"`
	Name      string             `json:"name"`
	Group     string             `json:"group,omitempty"`
	Unit      string             `json:"unit,omitempty"`
	Values    []float64          `json:"values"`
	Scores    map[string]float64 `json:"scores,omitempty"`
	Tags      []string           `json:"tags,omitempty"`
	ObservationContext
}

type TableColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Unit     string `json:"unit,omitempty"`
	Nullable bool   `json:"nullable,omitempty"`
}

type TableObservation struct {
	JobID     string           `json:"-"`
	AttemptID string           `json:"attempt_id,omitempty"`
	Name      string           `json:"name"`
	Subtype   string           `json:"subtype,omitempty"`
	Columns   []TableColumn    `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	Tags      []string         `json:"tags,omitempty"`
	Replace   bool             `json:"replace,omitempty"`
	ObservationContext
}

type TableRow struct {
	Cursor     int64          `json:"cursor"`
	Step       *int64         `json:"step,omitempty"`
	CapturedAt time.Time      `json:"timestamp"`
	Values     map[string]any `json:"values"`
}

type TablePage struct {
	AttemptID string         `json:"attempt_id"`
	Name      string         `json:"name"`
	Subtype   string         `json:"subtype"`
	Columns   []TableColumn  `json:"columns"`
	Tags      []string       `json:"tags,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Items     []TableRow     `json:"items"`
	Total     int64          `json:"total"`
	Next      *int64         `json:"next_cursor,omitempty"`
}

type ProgressState struct {
	AttemptID      string               `json:"attempt_id"`
	Simple         *ProgressObservation `json:"simple,omitempty"`
	Current        *ProgressObservation `json:"current,omitempty"`
	Milestones     []Milestone          `json:"milestones"`
	Reached        []string             `json:"reached"`
	GlobalProgress *float64             `json:"global_progress,omitempty"`
	UpdatedAt      *time.Time           `json:"updated_at,omitempty"`
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
