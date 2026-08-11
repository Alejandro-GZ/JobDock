package domain

import "time"

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

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
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
	CreatedAt         time.Time         `json:"created_at"`
}

type Event struct {
	ID        int64          `json:"id"`
	JobID     string         `json:"job_id"`
	Sequence  int64          `json:"sequence"`
	Type      string         `json:"type"`
	Status    JobStatus      `json:"status,omitempty"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

// ResourceSample is deliberately limited to normalized scalar resource data.
// Docker Stats responses must never cross this boundary or reach persistence.
type ResourceSample struct {
	JobID                     string    `json:"-"`
	CapturedAt                time.Time `json:"-"`
	ResolutionSeconds         int       `json:"-"`
	SampleCount               int       `json:"-"`
	CPUMillis                 int64     `json:"cpu_millis"`
	MemoryBytes               int64     `json:"memory_bytes"`
	GPUUtilizationBasisPoints *int64    `json:"gpu_utilization_basis_points,omitempty"`
	GPUMemoryBytes            *int64    `json:"gpu_memory_bytes,omitempty"`
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
