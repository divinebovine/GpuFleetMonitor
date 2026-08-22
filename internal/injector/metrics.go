package injector

import (
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsRecorder records the current value of a DCGM field for a specific
// GPU entity, one call per field per entity per tick. Record always reflects
// the latest value given — it does not accumulate deltas, so callers must
// pass the current absolute reading each time, not an incremental amount.
// Implementations must be safe for concurrent use: the HTTP handler serving
// /metrics reads while the tick loop writes.
type MetricsRecorder interface {
	Record(entityID uint, fieldID uint16, value float64)
}

type MetricsRecorderImpl struct {
	mu            sync.RWMutex
	modelByEntity map[uint]string
	values        map[uint16]map[uint]float64 // fieldID -> entityID -> current value
	descs         map[uint16]*prometheus.Desc
	valueTypes    map[uint16]prometheus.ValueType // counter, gauge, etc.
}

// NewMetricsRecorder builds a MetricsRecorder that is also a valid
// prometheus.Collector, ready to be registered with a prometheus.Registry
// and mounted at /metrics via promhttp.HandlerFor. It only reports the fixed
// set of DCGM fields declared in metricsConfig — Record silently ignores any
// field ID not present there. modelByEntity maps each entity ID to its GPU
// model, supplying the "modelName" label (matching dcgm-exporter's own
// convention, so dashboards/queries built against real dcgm-exporter output
// still work unmodified).
func NewMetricsRecorder(modelByEntity map[uint]string) *MetricsRecorderImpl {
	return &MetricsRecorderImpl{
		modelByEntity: modelByEntity,
		values:        initValues(metricsConfig),
		descs:         initDescs(metricsConfig),
		valueTypes:    initValueTypes(metricsConfig),
	}
}

func (m *MetricsRecorderImpl) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range m.descs {
		ch <- d
	}
}

func (m *MetricsRecorderImpl) Collect(ch chan<- prometheus.Metric) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for fieldID, byEntityID := range m.values {
		desc := m.descs[fieldID]
		vt := m.valueTypes[fieldID]
		for entityID, value := range byEntityID {
			ch <- prometheus.MustNewConstMetric(desc, vt, value, strconv.Itoa(int(entityID)), m.modelByEntity[entityID])
		}
	}
}

func (m *MetricsRecorderImpl) Record(entityID uint, fieldID uint16, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.values[fieldID] == nil {
		return // untracked field
	}

	m.values[fieldID][entityID] = value
}
