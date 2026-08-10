# JobDock Python SDK

The SDK adds optional progress, scalar metrics, parameters, structured events, artifact registration, and cooperative cancellation to a JobDock job. It has no runtime dependencies outside the Python standard library.

```python
from jobdock import current_job

job = current_job()
job.progress(0.5)
job.metric("loss", 0.42, step=10)

if job.should_stop():
    save_checkpoint()
```

Outside JobDock, `current_job()` returns a no-op object. Use `current_job(required=True)` when missing execution context should be an error.

