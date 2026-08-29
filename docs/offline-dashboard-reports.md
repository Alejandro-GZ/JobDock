# Offline dashboard reports

JobDock can export one or more dashboards from a job attempt as a single self-contained HTML report. The file embeds its dashboard configuration, bounded datasets, styles, and visualization runtime. It can be opened without JobDock, authentication, or network access.

Open a job, select the attempt and the **Metrics** view, then choose **Export report**. Select the dashboards to include and download the generated HTML file. Each dashboard keeps its own layout and palette and appears as a navigable report tab.

Reports are immutable snapshots. For a running attempt, the report contains only data available at its `generated_at` timestamp and identifies that condition in its header. Report provenance includes the JobDock version, report schema, job and attempt identifiers, dashboard identifiers, status, and relevant timestamps.

## Export policy

To keep report generation predictable and memory bounded, JobDock applies these limits:

- 2,000 automatically downsampled points per metric or resource series.
- 500 deterministically sampled rows per table-backed source.
- 512 samples per distribution snapshot.
- Matrices aggregated to at most 64 × 64 cells when their type supports aggregation.
- 500 confirmed checkpoints.
- The final 1 MiB of each selected log stream.
- 50 MiB for the complete HTML file and 30 seconds for generation.

Sampling, truncation, missing observations, and matrix aggregation are listed both in the affected report and under **Export notes**. A report that would exceed the hard 50 MiB limit is rejected instead of being silently incomplete.

## Security properties

The report does not contain browser sessions, personal access tokens, job tokens, signed URLs, or registry credentials. Job-reported strings are serialized as data and rendered as text. A restrictive Content Security Policy disables network connections, forms, frames, objects, base URLs, and arbitrary scripts. The report runtime and datasets are embedded directly in the file.

The export endpoint is `POST /api/v1/jobs/{jobId}/dashboard-reports` and requires a browser session plus CSRF protection. Dashboard selection is scoped to the authenticated user's dashboards for the requested job.
