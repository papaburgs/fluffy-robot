# Combined Collector and UI

This is a switch back to the combined collector and UI with a local file storage.

The logic for the collector is the same as previous versions.

The backend is file based. Each reset will have a directory, in that directory will be various files outlined later.

## Configuration

*   `FLUFFY_GATE_BUCKET_SIZE`: API burst bucket size (default: 20).
*   `FLUFFY_AGENT_IGNORE_FILTERS`: Regex patterns for excluding agents.
