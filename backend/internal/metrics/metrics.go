package metrics

import "github.com/prometheus/client_golang/prometheus"

type Registry struct {
	RequestCount      *prometheus.CounterVec
	RequestLatency    *prometheus.HistogramVec
	AuthFailures      prometheus.Counter
	UploadCount       prometheus.Counter
	DownloadCount     prometheus.Counter
	BytesTransferred  *prometheus.CounterVec
	WorkerJobsHandled *prometheus.CounterVec
}

func New() *Registry {
	return &Registry{
		RequestCount:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: "harborshield_requests_total", Help: "HTTP requests handled by route and status"}, []string{"route", "method", "status"}),
		RequestLatency:    prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "harborshield_request_latency_seconds", Help: "HTTP latency by route and method", Buckets: prometheus.DefBuckets}, []string{"route", "method"}),
		AuthFailures:      prometheus.NewCounter(prometheus.CounterOpts{Name: "harborshield_auth_failures_total", Help: "Authentication failures"}),
		UploadCount:       prometheus.NewCounter(prometheus.CounterOpts{Name: "harborshield_uploads_total", Help: "Uploaded objects"}),
		DownloadCount:     prometheus.NewCounter(prometheus.CounterOpts{Name: "harborshield_downloads_total", Help: "Downloaded objects"}),
		BytesTransferred:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "harborshield_bytes_transferred_total", Help: "Bytes transferred by direction"}, []string{"direction"}),
		WorkerJobsHandled: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "harborshield_worker_jobs_total", Help: "Worker jobs handled by type and status"}, []string{"job_type", "status"}),
	}
}

func (r *Registry) MustRegister() {
	prometheus.MustRegister(r.RequestCount, r.RequestLatency, r.AuthFailures, r.UploadCount, r.DownloadCount, r.BytesTransferred, r.WorkerJobsHandled)
}
