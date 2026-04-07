package metrics

import (
	"expvar"
)

var (
	CollectorAPICalls           = expvar.NewInt("collector_api_calls_total")
	CollectorAPI429Retries      = expvar.NewInt("collector_api_retries_429_total")
	CollectorAPIOtherRetries    = expvar.NewInt("collector_api_retries_other_total")
	CollectorAgentUpdates       = expvar.NewInt("collector_agent_updates_total")
	CollectorJumpgateUpdates    = expvar.NewInt("collector_jumpgate_updates_total")
	CollectorConstructionChecks = expvar.NewInt("collector_construction_checks_total")
	CollectorResetDetections    = expvar.NewInt("collector_reset_detections_total")
	CollectorLastTimestamp      = expvar.NewInt("collector_last_update_timestamp")

	GateQueueLength = expvar.NewInt("gate_queue_length")
	GateT1Requests  = expvar.NewInt("gate_requests_t1_total")
	GateT60Requests = expvar.NewInt("gate_requests_t60_total")
	GateBlocked     = expvar.NewInt("gate_blocked_total")
	GateLockCount   = expvar.NewInt("gate_lock_count")

	DatastoreWrites      = expvar.NewInt("datastore_write_operations_total")
	DatastoreReads       = expvar.NewInt("datastore_read_operations_total")
	DatastoreCacheResets = expvar.NewInt("datastore_cache_resets_total")
)
