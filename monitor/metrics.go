package monitor

import (
	"sync"
	"time"

	"github.com/crtsh/ctsubmit/config"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var submissionOutcomeCounter = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: config.ApplicationNamespace,
	Subsystem: "submission",
	Name:      "outcome_total",
	Help:      "Total submission outcomes per log, by outcome.",
}, []string{"url", "outcome"})

// RecordSubmissionOutcome increments the Prometheus counter for a submission outcome.
func RecordSubmissionOutcome(submissionURL string, outcome string) {
	submissionOutcomeCounter.WithLabelValues(submissionURL, outcome).Inc()
	recordOutcomeSample(submissionURL, outcome)
}

// outcomeSample is a single recorded outcome with its timestamp.
type outcomeSample struct {
	at      time.Time
	success bool
}

var (
	outcomeSamples      = make(map[string][]outcomeSample)
	outcomeSamplesMutex sync.Mutex
)

func recordOutcomeSample(submissionURL string, outcome string) {
	// Cancelled submissions never received a response, so don't count them.
	if outcome == "cancelled" {
		return
	}
	outcomeSamplesMutex.Lock()
	defer outcomeSamplesMutex.Unlock()
	now := time.Now()
	cutoff := now.Add(-responseTimeWindow)
	samples := outcomeSamples[submissionURL]
	i := 0
	for i < len(samples) && samples[i].at.Before(cutoff) {
		i++
	}
	outcomeSamples[submissionURL] = append(samples[i:], outcomeSample{at: now, success: outcome == "success"})
}

// GetRecentOutcomeCounts returns the number of successful and failed responses in the last 30s.
func GetRecentOutcomeCounts(submissionURL string) (successes, failures int) {
	outcomeSamplesMutex.Lock()
	defer outcomeSamplesMutex.Unlock()
	cutoff := time.Now().Add(-responseTimeWindow)
	samples := outcomeSamples[submissionURL]
	i := 0
	for i < len(samples) && samples[i].at.Before(cutoff) {
		i++
	}
	samples = samples[i:]
	outcomeSamples[submissionURL] = samples
	for _, s := range samples {
		if s.success {
			successes++
		} else {
			failures++
		}
	}
	return
}

var submissionResponseTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: config.ApplicationNamespace,
	Subsystem: "submission",
	Name:      "response_seconds",
	Help:      "Per-log submission response time in seconds (excludes cancelled submissions).",
	Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
}, []string{"url"})

// RecordSubmissionResponseTime observes a response time for the given log.
func RecordSubmissionResponseTime(submissionURL string, d time.Duration) {
	submissionResponseTime.WithLabelValues(submissionURL).Observe(d.Seconds())
	recordResponseTimeSample(submissionURL, d)
}

// responseTimeSample is a single recorded response time with its timestamp.
type responseTimeSample struct {
	at       time.Time
	duration time.Duration
}

var (
	responseTimeSamples      = make(map[string][]responseTimeSample)
	responseTimeSamplesMutex sync.Mutex
)

const responseTimeWindow = 30 * time.Second

func recordResponseTimeSample(submissionURL string, d time.Duration) {
	responseTimeSamplesMutex.Lock()
	defer responseTimeSamplesMutex.Unlock()
	now := time.Now()
	cutoff := now.Add(-responseTimeWindow)
	samples := responseTimeSamples[submissionURL]
	// Drop samples older than the window.
	i := 0
	for i < len(samples) && samples[i].at.Before(cutoff) {
		i++
	}
	responseTimeSamples[submissionURL] = append(samples[i:], responseTimeSample{at: now, duration: d})
}

// GetAvgResponseTime returns the average response time over the last 30s for a log.
func GetAvgResponseTime(submissionURL string) (time.Duration, bool) {
	responseTimeSamplesMutex.Lock()
	defer responseTimeSamplesMutex.Unlock()
	cutoff := time.Now().Add(-responseTimeWindow)
	samples := responseTimeSamples[submissionURL]
	// Drop stale samples.
	i := 0
	for i < len(samples) && samples[i].at.Before(cutoff) {
		i++
	}
	samples = samples[i:]
	responseTimeSamples[submissionURL] = samples
	if len(samples) == 0 {
		return 0, false
	}
	var total time.Duration
	for _, s := range samples {
		total += s.duration
	}
	return total / time.Duration(len(samples)), true
}
