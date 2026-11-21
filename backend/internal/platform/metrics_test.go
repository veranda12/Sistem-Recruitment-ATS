package platform

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry gabungan yang dipakai ServeMetrics harus dapat dikumpulkan tanpa metrik ganda.
func TestMetricsGatherWithoutDuplicates(t *testing.T) {
	reg := prometheus.NewRegistry()
	ExamOps.WithLabelValues("save", "commit").Inc()
	h := promhttp.HandlerFor(prometheus.Gatherers{prometheus.DefaultGatherer, reg}, promhttp.HandlerOpts{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `ats_exam_operations_total{op="save",outcome="commit"} 1`) {
		t.Fatalf("metrics: %d\n%s", rec.Code, rec.Body.String()[:200])
	}
}
