package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Update is the message inputs send to report violation state.
type Update struct {
	Source             string
	Policy             string
	Decision           string
	ViolationNamespace string
	Kind               string
	Name               string
	Active             bool // true=upsert, false=delete
}

// labelKey uniquely identifies a violation by its full gauge label set.
type labelKey [6]string

func keyOf(u Update) labelKey {
	return labelKey{u.Source, u.Policy, u.Decision, u.ViolationNamespace, u.Kind, u.Name}
}

// Handler owns the updates channel, maintains violation state, and serves Prometheus metrics.
// Run must be called in a goroutine to process updates and run GC.
type Handler struct {
	entries map[labelKey]time.Time // key -> last seen; only accessed from Run goroutine
	ch      chan Update
	gauge   *prometheus.GaugeVec
	scrape  http.Handler
}

func NewHandler() *Handler {
	reg := prometheus.NewRegistry()
	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "violation",
			Help: "Active policy violations discovered by any input.",
		},
		[]string{"source", "policy", "decision", "violation_namespace", "kind", "name"},
	)
	reg.MustRegister(gauge)

	return &Handler{
		entries: make(map[labelKey]time.Time),
		ch:      make(chan Update, 1024),
		gauge:   gauge,
		scrape:  promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
	}
}

// Updates returns the send-only channel inputs use to report violations.
func (h *Handler) Updates() chan<- Update {
	return h.ch
}

// PrometheusHandler returns the HTTP handler for Prometheus scrapes.
func (h *Handler) PrometheusHandler() http.Handler {
	return h.scrape
}

// Run processes incoming updates and periodically garbage-collects entries not
// refreshed within gcInterval. It blocks until ctx is cancelled.
func (h *Handler) Run(ctx context.Context, gcInterval time.Duration) {
	ticker := time.NewTicker(gcInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case u := <-h.ch:
			h.apply(u)
		case t := <-ticker.C:
			h.gc(t, gcInterval)
		}
	}
}

func (h *Handler) apply(u Update) {
	k := keyOf(u)
	if u.Active {
		h.entries[k] = time.Now()
		h.gauge.WithLabelValues(k[0], k[1], k[2], k[3], k[4], k[5]).Set(1)
	} else {
		delete(h.entries, k)
		h.gauge.DeleteLabelValues(k[0], k[1], k[2], k[3], k[4], k[5])
	}
}

func (h *Handler) gc(now time.Time, interval time.Duration) {
	for k, lastSeen := range h.entries {
		if now.Sub(lastSeen) > interval {
			h.gauge.DeleteLabelValues(k[0], k[1], k[2], k[3], k[4], k[5])
			delete(h.entries, k)
		}
	}
}
