//go:build !disable_kopia

/*
Copyright 2024 The VolSync authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package kopia

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	kopiaMetricsNamespace = "volsync_kopia"
)

// kopiaMetrics holds references to the Kopia-specific metric vectors
type kopiaMetrics struct {
	// Backup/Restore Performance Metrics
	OperationDuration *prometheus.SummaryVec
	DataProcessed     *prometheus.SummaryVec
	DataTransferRate  *prometheus.SummaryVec
	CompressionRatio  *prometheus.SummaryVec
	OperationSuccess  *prometheus.CounterVec
	OperationFailure  *prometheus.CounterVec

	// Repository Health Metrics
	RepositoryConnectivity *prometheus.GaugeVec
	MaintenanceOperations  *prometheus.CounterVec
	MaintenanceDuration    *prometheus.SummaryVec
	RepositorySize         *prometheus.GaugeVec
	RepositoryObjects      *prometheus.GaugeVec

	// Snapshot Management Metrics
	SnapshotCount           *prometheus.GaugeVec
	SnapshotCreationSuccess *prometheus.CounterVec
	SnapshotCreationFailure *prometheus.CounterVec
	SnapshotSize            *prometheus.SummaryVec
	DeduplicationRatio      *prometheus.SummaryVec
	RetentionCompliance     *prometheus.GaugeVec

	// Cache and Performance Metrics
	CacheHitRate       *prometheus.GaugeVec
	CacheSize          *prometheus.GaugeVec
	CacheType          *prometheus.GaugeVec
	ParallelOperations *prometheus.GaugeVec
	JobRetries         *prometheus.CounterVec
	QueueDepth         *prometheus.GaugeVec

	// Policy and Configuration Metrics
	PolicyCompliance      *prometheus.GaugeVec
	ConfigurationErrors   *prometheus.CounterVec
	CustomActionsExecuted *prometheus.CounterVec
	CustomActionsDuration *prometheus.SummaryVec
}

var (
	kopiaMetricLabels = []string{
		"obj_name",      // Name of the replication CR
		"obj_namespace", // Namespace containing the CR
		"role",          // Direction: "source" or "destination"
		"operation",     // Type of operation: "backup", "restore", "maintenance"
		"repository",    // Repository name for grouping
	}

	// Backup/Restore Performance Metrics
	operationDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "operation_duration_seconds",
			Namespace:  kopiaMetricsNamespace,
			Help:       "Duration of Kopia operations (backup, restore, maintenance) in seconds",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			MaxAge:     24 * time.Hour,
		},
		kopiaMetricLabels,
	)

	dataProcessed = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "data_processed_bytes",
			Namespace:  kopiaMetricsNamespace,
			Help:       "Amount of data processed during Kopia operations in bytes",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			MaxAge:     24 * time.Hour,
		},
		kopiaMetricLabels,
	)

	dataTransferRate = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "data_transfer_rate_bytes_per_second",
			Namespace:  kopiaMetricsNamespace,
			Help:       "Data transfer rate during Kopia operations in bytes per second",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			MaxAge:     24 * time.Hour,
		},
		kopiaMetricLabels,
	)

	compressionRatio = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "compression_ratio",
			Namespace:  kopiaMetricsNamespace,
			Help:       "Compression ratio achieved during backup operations (compressed_size/original_size)",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			MaxAge:     24 * time.Hour,
		},
		kopiaMetricLabels,
	)

	operationSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "operation_success_total",
			Namespace: kopiaMetricsNamespace,
			Help:      "Total number of successful Kopia operations",
		},
		kopiaMetricLabels,
	)

	operationFailure = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "operation_failure_total",
			Namespace: kopiaMetricsNamespace,
			Help:      "Total number of failed Kopia operations with failure reason",
		},
		append(kopiaMetricLabels, "failure_reason"),
	)

	// Repository Health Metrics
	repositoryConnectivity = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "repository_connectivity",
			Namespace: kopiaMetricsNamespace,
			Help:      "Repository connectivity status (1 if connected, 0 if not)",
		},
		kopiaMetricLabels,
	)

	maintenanceOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "maintenance_operations_total",
			Namespace: kopiaMetricsNamespace,
			Help:      "Total number of repository maintenance operations performed",
		},
		append(kopiaMetricLabels, "maintenance_type"),
	)

	maintenanceDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "maintenance_duration_seconds",
			Namespace:  kopiaMetricsNamespace,
			Help:       "Duration of repository maintenance operations in seconds",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			MaxAge:     7 * 24 * time.Hour, // Keep for a week since maintenance is less frequent
		},
		append(kopiaMetricLabels, "maintenance_type"),
	)

	repositorySize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "repository_size_bytes",
			Namespace: kopiaMetricsNamespace,
			Help:      "Total size of the Kopia repository in bytes",
		},
		kopiaMetricLabels,
	)

	repositoryObjects = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "repository_objects_total",
			Namespace: kopiaMetricsNamespace,
			Help:      "Total number of objects in the Kopia repository",
		},
		kopiaMetricLabels,
	)

	// Snapshot Management Metrics
	snapshotCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "snapshot_count",
			Namespace: kopiaMetricsNamespace,
			Help:      "Current number of snapshots in the repository",
		},
		kopiaMetricLabels,
	)

	snapshotCreationSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "snapshot_creation_success_total",
			Namespace: kopiaMetricsNamespace,
			Help:      "Total number of successful snapshot creations",
		},
		kopiaMetricLabels,
	)

	snapshotCreationFailure = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "snapshot_creation_failure_total",
			Namespace: kopiaMetricsNamespace,
			Help:      "Total number of failed snapshot creations",
		},
		append(kopiaMetricLabels, "failure_reason"),
	)

	snapshotSize = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "snapshot_size_bytes",
			Namespace:  kopiaMetricsNamespace,
			Help:       "Size of individual snapshots in bytes",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			MaxAge:     24 * time.Hour,
		},
		kopiaMetricLabels,
	)

	deduplicationRatio = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "deduplication_ratio",
			Namespace:  kopiaMetricsNamespace,
			Help:       "Deduplication efficiency ratio (deduplicated_size/original_size)",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			MaxAge:     24 * time.Hour,
		},
		kopiaMetricLabels,
	)

	retentionCompliance = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "retention_compliance",
			Namespace: kopiaMetricsNamespace,
			Help:      "Retention policy compliance status (1 if compliant, 0 if not)",
		},
		append(kopiaMetricLabels, "retention_type"), // hourly, daily, weekly, monthly, yearly
	)

	// Cache and Performance Metrics
	cacheHitRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "cache_hit_rate",
			Namespace: kopiaMetricsNamespace,
			Help:      "Cache hit rate as a percentage (0-100)",
		},
		kopiaMetricLabels,
	)

	cacheSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "cache_size_bytes",
			Namespace: kopiaMetricsNamespace,
			Help:      "Current size of the Kopia cache in bytes",
		},
		kopiaMetricLabels,
	)

	cacheType = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "cache_type",
			Namespace: kopiaMetricsNamespace,
			Help:      "Cache type being used (1 for the active type, 0 for inactive)",
		},
		append(kopiaMetricLabels, "cache_type"), // pvc, emptydir
	)

	parallelOperations = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "parallel_operations_active",
			Namespace: kopiaMetricsNamespace,
			Help:      "Number of currently active parallel operations",
		},
		kopiaMetricLabels,
	)

	jobRetries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "job_retries_total",
			Namespace: kopiaMetricsNamespace,
			Help:      "Total number of job retries due to failures",
		},
		append(kopiaMetricLabels, "retry_reason"),
	)

	queueDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "queue_depth",
			Namespace: kopiaMetricsNamespace,
			Help:      "Current depth of the operation queue",
		},
		kopiaMetricLabels,
	)

	// Policy and Configuration Metrics
	policyCompliance = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "policy_compliance",
			Namespace: kopiaMetricsNamespace,
			Help:      "Policy compliance status (1 if compliant, 0 if not)",
		},
		append(kopiaMetricLabels, "policy_type"), // global, retention, compression
	)

	configurationErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "configuration_errors_total",
			Namespace: kopiaMetricsNamespace,
			Help:      "Total number of configuration errors encountered",
		},
		append(kopiaMetricLabels, "error_type"),
	)

	customActionsExecuted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "custom_actions_executed_total",
			Namespace: kopiaMetricsNamespace,
			Help:      "Total number of custom actions executed",
		},
		append(kopiaMetricLabels, "action_type"), // before_snapshot, after_snapshot
	)

	customActionsDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "custom_actions_duration_seconds",
			Namespace:  kopiaMetricsNamespace,
			Help:       "Duration of custom action execution in seconds",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			MaxAge:     24 * time.Hour,
		},
		append(kopiaMetricLabels, "action_type"),
	)
)

func newKopiaMetrics() kopiaMetrics {
	return kopiaMetrics{
		// Backup/Restore Performance Metrics
		OperationDuration: operationDuration,
		DataProcessed:     dataProcessed,
		DataTransferRate:  dataTransferRate,
		CompressionRatio:  compressionRatio,
		OperationSuccess:  operationSuccess,
		OperationFailure:  operationFailure,

		// Repository Health Metrics
		RepositoryConnectivity: repositoryConnectivity,
		MaintenanceOperations:  maintenanceOperations,
		MaintenanceDuration:    maintenanceDuration,
		RepositorySize:         repositorySize,
		RepositoryObjects:      repositoryObjects,

		// Snapshot Management Metrics
		SnapshotCount:           snapshotCount,
		SnapshotCreationSuccess: snapshotCreationSuccess,
		SnapshotCreationFailure: snapshotCreationFailure,
		SnapshotSize:            snapshotSize,
		DeduplicationRatio:      deduplicationRatio,
		RetentionCompliance:     retentionCompliance,

		// Cache and Performance Metrics
		CacheHitRate:       cacheHitRate,
		CacheSize:          cacheSize,
		CacheType:          cacheType,
		ParallelOperations: parallelOperations,
		JobRetries:         jobRetries,
		QueueDepth:         queueDepth,

		// Policy and Configuration Metrics
		PolicyCompliance:      policyCompliance,
		ConfigurationErrors:   configurationErrors,
		CustomActionsExecuted: customActionsExecuted,
		CustomActionsDuration: customActionsDuration,
	}
}

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		// Backup/Restore Performance Metrics
		operationDuration,
		dataProcessed,
		dataTransferRate,
		compressionRatio,
		operationSuccess,
		operationFailure,

		// Repository Health Metrics
		repositoryConnectivity,
		maintenanceOperations,
		maintenanceDuration,
		repositorySize,
		repositoryObjects,

		// Snapshot Management Metrics
		snapshotCount,
		snapshotCreationSuccess,
		snapshotCreationFailure,
		snapshotSize,
		deduplicationRatio,
		retentionCompliance,

		// Cache and Performance Metrics
		cacheHitRate,
		cacheSize,
		cacheType,
		parallelOperations,
		jobRetries,
		queueDepth,

		// Policy and Configuration Metrics
		policyCompliance,
		configurationErrors,
		customActionsExecuted,
		customActionsDuration,
	)
}
