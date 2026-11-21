// Package cvreview mengintegrasikan pembaca CV ke ATS: run berantrean, worker khusus, review saran
// dengan bukti, dan penerapan ke profil kandidat setelah disetujui.
package cvreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"ats/db/sqlc"
	"ats/internal/cvparse"
	"ats/internal/platform"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	TaskParse   = "cv:parse"
	QueueName   = "cv" // antrean khusus CV, terpisah dari outbox/antrean ujian
	leaseSecond = 45
)

// ConfigVersion ikut kunci idempotensi: perubahan parser atau batas membuat run baru.
func ConfigVersion(lim cvparse.Limits) string {
	return fmt.Sprintf("%s/%dMiB-%dp-%ddpi", cvparse.ParserVersion, lim.MaxBytes>>20, lim.MaxPages, lim.OCRDPI)
}

// ArtifactDir: direktori artefak privat (preview halaman) untuk satu run dan fence.
func ArtifactDir(objectDir string, runID uuid.UUID, fence int64) string {
	return filepath.Join(objectDir, "cvparse", runID.String(), fmt.Sprintf("f%d", fence))
}

var (
	mRuns = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ats_cv_parse_runs_total", Help: "Run pembacaan CV per hasil dan format"},
		[]string{"format", "outcome", "reason"})
	mProcess = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "ats_cv_parse_process_seconds", Help: "Waktu proses (tanpa antre)",
		Buckets: []float64{0.25, 0.5, 1, 2, 5, 10, 20, 30, 60, 120}}, []string{"format"})
	mQueue = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "ats_cv_parse_queue_seconds", Help: "Waktu tunggu di antrean",
		Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60, 120, 300, 900}})
	mPages      = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ats_cv_parse_pages_total", Help: "Halaman per metode baca"}, []string{"method", "status"})
	metricsOnce sync.Once
)

func registerMetrics() {
	metricsOnce.Do(func() { prometheus.MustRegister(mRuns, mProcess, mQueue, mPages) })
}

// Processor menjalankan satu run dengan lease + fence.
type Processor struct {
	Pool      *pgxpool.Pool
	Q         *sqlc.Queries
	ObjectDir string
	WorkDir   string
	Tools     cvparse.Tools
	Limits    cvparse.Limits
	Owner     string // identitas worker untuk lease
}

// Run mengklaim lalu memproses run. Mengembalikan nil bila run tidak jatuh tempo atau sudah dipegang worker lain.
func (pr *Processor) Run(ctx context.Context, runID uuid.UUID) error {
	owner := pr.Owner
	run, err := pr.Q.ClaimCVParseRun(ctx, sqlc.ClaimCVParseRunParams{Owner: &owner, LeaseSeconds: leaseSecond, ID: runID})
	if platform.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if run.QueueMs != nil {
		mQueue.Observe(float64(*run.QueueMs) / 1000)
	}
	doc, err := pr.Q.GetDocument(ctx, run.DocumentID)
	if err != nil {
		return err
	}
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Progres dibaca heartbeat (bukan ditulis per halaman) agar beban DB rendah.
	var stage atomic.Value
	stage.Store("detecting")
	var done, total atomic.Int32
	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-jobCtx.Done():
				return
			case <-t.C:
				var tot *int32
				if v := total.Load(); v > 0 {
					tot = &v
				}
				n, err := pr.Q.HeartbeatCVParseRun(context.WithoutCancel(jobCtx), sqlc.HeartbeatCVParseRunParams{LeaseSeconds: leaseSecond,
					Stage: stage.Load().(string), PagesDone: done.Load(), PagesTotal: tot, ID: run.ID, Fence: run.Fence})
				if err == nil && n == 0 {
					// fence hilang (dibatalkan/diambil alih): hentikan pekerjaan
					slog.Warn("cv parse kehilangan lease", "run_id", run.ID.String())
					cancel()
					return
				}
			}
		}
	}()
	progress := func(st string, d, t int) {
		stage.Store(st)
		done.Store(int32(d))
		total.Store(int32(t))
	}
	src := filepath.Join(pr.ObjectDir, filepath.FromSlash(doc.ObjectKey))
	outDir := ArtifactDir(pr.ObjectDir, run.ID, run.Fence)
	t0 := time.Now()
	res, perr := cvparse.Process(jobCtx, pr.Tools, pr.Limits, src, pr.WorkDir, outDir, progress)
	cancel()
	<-hbDone
	format := "unknown"
	if res != nil {
		format = res.Manifest.Format
	}
	mProcess.WithLabelValues(format).Observe(time.Since(t0).Seconds())
	bg := context.WithoutCancel(ctx)

	var fl *cvparse.Failure
	switch {
	case perr == nil || (errors.As(perr, &fl) && res != nil && res.Doc != nil && !fl.Transient && anyPageOK(res.Doc)):
		return pr.persist(bg, run, res, outDir)
	case errors.As(perr, &fl) && fl.Transient:
		backoff := int32(math.Min(600, 30*math.Pow(2, float64(run.Attempt-1))))
		n, err := pr.Q.RetryCVParseRun(bg, sqlc.RetryCVParseRunParams{FailureReason: ptr(string(fl.Reason)), BackoffSeconds: backoff, ID: run.ID, Fence: run.Fence})
		mRuns.WithLabelValues(format, "retry", string(fl.Reason)).Inc()
		if err == nil && n == 0 {
			_ = os.RemoveAll(outDir)
		}
		return err
	case errors.As(perr, &fl):
		mRuns.WithLabelValues(format, "failed", string(fl.Reason)).Inc()
		var manifest []byte
		if res != nil {
			manifest, _ = json.Marshal(res.Manifest)
		}
		n, err := pr.Q.FinishCVParseRun(bg, sqlc.FinishCVParseRunParams{Status: "failed", Manifest: manifest, FailureReason: ptr(string(fl.Reason)), ID: run.ID, Fence: run.Fence})
		if err == nil && n == 0 {
			_ = os.RemoveAll(outDir)
		}
		if res != nil && res.Doc != nil && err == nil && n == 1 {
			// halaman yang gagal tetap tercatat agar kandidat melihat alasannya
			for _, p := range res.Doc.Pages {
				_ = pr.Q.InsertCVParsePage(bg, pageParams(run.ID, run.Fence, p))
			}
		}
		return err
	default:
		if ctx.Err() != nil {
			return ctx.Err() // worker berhenti; lease habis -> diambil alih
		}
		backoff := int32(math.Min(600, 30*math.Pow(2, float64(run.Attempt-1))))
		_, err := pr.Q.RetryCVParseRun(bg, sqlc.RetryCVParseRunParams{FailureReason: ptr("internal_error"), BackoffSeconds: backoff, ID: run.ID, Fence: run.Fence})
		slog.Error("cv parse error internal", "run_id", run.ID.String(), "err", perr.Error())
		mRuns.WithLabelValues(format, "retry", "internal_error").Inc()
		return err
	}
}

func anyPageOK(d *cvparse.Doc) bool {
	for _, p := range d.Pages {
		if p.Status == "ok" {
			return true
		}
	}
	return false
}

func ptr[T any](v T) *T { return &v }

func pageParams(run uuid.UUID, fence int64, p *cvparse.Page) sqlc.InsertCVParsePageParams {
	var img *string
	if p.Image != "" {
		img = ptr(fmt.Sprintf("f%d/%s", fence, p.Image))
	}
	var fr *string
	if p.Failure != "" {
		fr = ptr(p.Failure)
	}
	return sqlc.InsertCVParsePageParams{RunID: run, PageNo: int32(p.No), Method: p.Method, Reason: p.Reason, Status: p.Status, FailureReason: fr,
		Width: ptr(float32(p.W)), Height: ptr(float32(p.H)), Scale: ptr(float32(p.Scale)), ImageName: img}
}

// persist menulis hasil dalam satu transaksi; gagal bila fence sudah berganti atau dokumen dihapus.
func (pr *Processor) persist(ctx context.Context, run sqlc.CvParseRun, res *cvparse.Result, outDir string) error {
	ex := cvparse.Extract(res.Doc)
	status := "ready_for_review"
	for _, p := range res.Doc.Pages {
		mPages.WithLabelValues(p.Method, p.Status).Inc()
		if p.Status != "ok" {
			status = "partial"
		}
	}
	manifest, _ := json.Marshal(res.Manifest)
	ir, _ := json.Marshal(res.Doc)
	pages := int32(len(res.Doc.Pages))
	err := platform.WithTx(ctx, pr.Pool, func(tx pgx.Tx) error {
		q := pr.Q.WithTx(tx)
		n, err := q.FinishCVParseRun(ctx, sqlc.FinishCVParseRunParams{Status: status, Manifest: manifest, PagesTotal: &pages, ID: run.ID, Fence: run.Fence})
		if err != nil {
			return err
		}
		if n == 0 {
			return errStale
		}
		for _, p := range res.Doc.Pages {
			if err := q.InsertCVParsePage(ctx, pageParams(run.ID, run.Fence, p)); err != nil {
				return err
			}
		}
		if err := q.InsertCVArtifact(ctx, sqlc.InsertCVArtifactParams{RunID: run.ID, Ir: ir}); err != nil {
			return err
		}
		evID := map[string]uuid.UUID{}
		for i, s := range ex.Suggestions {
			for _, lid := range s.Evidence {
				if _, ok := evID[lid]; ok {
					continue
				}
				l := res.Doc.Line(lid)
				if l == nil {
					continue
				}
				var bbox []float32
				if l.Source != "docx" {
					bbox = []float32{float32(l.B.X0), float32(l.B.Y0), float32(l.B.X1), float32(l.B.Y1)}
				}
				snippet := l.Text()
				if len([]rune(snippet)) > 2000 {
					snippet = string([]rune(snippet)[:2000])
				}
				id, err := q.InsertCVEvidence(ctx, sqlc.InsertCVEvidenceParams{RunID: run.ID, LineID: lid, PageNo: int32(l.Page), Bbox: bbox,
					Snippet: snippet, Source: l.Source, Locator: l.Locator})
				if err != nil {
					return err
				}
				evID[lid] = id
			}
			norm, _ := json.Marshal(s.Normalized)
			sid, err := q.InsertCVSuggestion(ctx, sqlc.InsertCVSuggestionParams{RunID: run.ID, Path: s.Path, Section: s.Section, EntryKey: s.EntryKey,
				Field: s.Field, RawValue: s.Raw, Normalized: norm, ReadDoubts: nonNil(s.ReadDoubt), AssocDoubts: nonNil(s.AssocDoubt), Sort: int32(i)})
			if err != nil {
				return err
			}
			for _, lid := range s.Evidence {
				if eid, ok := evID[lid]; ok {
					if err := q.LinkCVSuggestionEvidence(ctx, sqlc.LinkCVSuggestionEvidenceParams{SuggestionID: sid, EvidenceID: eid}); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if errors.Is(err, errStale) {
		_ = os.RemoveAll(outDir)
		slog.Warn("cv parse hasil dibuang: fence berganti atau dokumen dihapus", "run_id", run.ID.String())
		mRuns.WithLabelValues(res.Manifest.Format, "stale", "fence_lost").Inc()
		return nil
	}
	if err == nil {
		mRuns.WithLabelValues(res.Manifest.Format, status, "").Inc()
	}
	return err
}

var errStale = errors.New("fence berganti")

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ---- Antrean ----

// Enqueuer mengirim sinyal ke Asynq. PostgreSQL tetap sumber kebenaran: sinyal yang hilang dipulihkan dispatcher.
type Enqueuer struct {
	Client *asynq.Client
}

func NewEnqueuer(redisURL string) (*Enqueuer, error) {
	if redisURL == "" {
		return &Enqueuer{}, nil
	}
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, err
	}
	return &Enqueuer{Client: asynq.NewClient(opt)}, nil
}

// Signal: TaskID = run+fence sehingga sinyal ganda untuk klaim yang sama ditolak Redis.
func (e *Enqueuer) Signal(ctx context.Context, runID uuid.UUID, fence int64) error {
	if e == nil || e.Client == nil {
		return errors.New("redis tidak dikonfigurasi")
	}
	payload, _ := json.Marshal(map[string]string{"runId": runID.String()})
	_, err := e.Client.EnqueueContext(ctx, asynq.NewTask(TaskParse, payload), asynq.Queue(QueueName),
		asynq.TaskID(fmt.Sprintf("%s-%d", runID, fence)), asynq.MaxRetry(0), asynq.Timeout(10*time.Minute), asynq.Retention(0))
	if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (e *Enqueuer) Close() {
	if e != nil && e.Client != nil {
		_ = e.Client.Close()
	}
}

// Dispatcher memulihkan run dari PostgreSQL: mengirim ulang sinyal yang hilang, mengambil alih lease
// kedaluwarsa, mendead-letter run yang percobaannya habis. Tanpa Redis, run diproses langsung (fallback).
type Dispatcher struct {
	Q        *sqlc.Queries
	Enq      *Enqueuer
	Proc     *Processor
	Interval time.Duration
	Local    chan struct{} // semafor fallback lokal (concurrency)
}

func (d *Dispatcher) Run(ctx context.Context) {
	t := time.NewTicker(d.Interval)
	defer t.Stop()
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		if ids, err := d.Q.ExpireExhaustedCVRuns(ctx); err == nil {
			for _, id := range ids {
				slog.Warn("cv parse dead-letter: percobaan habis", "run_id", id.String())
				mRuns.WithLabelValues("unknown", "failed", "attempts_exhausted").Inc()
			}
		}
		due, err := d.Q.DueCVParseRuns(ctx, 50)
		if err == nil {
			for _, r := range due {
				if d.Enq.Signal(ctx, r.ID, r.Fence) == nil {
					_ = d.Q.MarkCVDispatched(ctx, r.ID)
					continue
				}
				// Redis tidak tersedia: proses langsung bila ada slot.
				select {
				case d.Local <- struct{}{}:
					_ = d.Q.MarkCVDispatched(ctx, r.ID)
					wg.Add(1)
					go func(id uuid.UUID) {
						defer wg.Done()
						defer func() { <-d.Local }()
						if err := d.Proc.Run(ctx, id); err != nil && ctx.Err() == nil {
							slog.Error("cv parse fallback", "run_id", id.String(), "err", err.Error())
						}
					}(r.ID)
				default:
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// AsynqHandler: handler task Asynq. Retry dikelola PostgreSQL (MaxRetry 0), jadi error tidak dikembalikan.
func AsynqHandler(p *Processor) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		var payload struct {
			RunID uuid.UUID `json:"runId"`
		}
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("%w: payload", asynq.SkipRetry)
		}
		if err := p.Run(ctx, payload.RunID); err != nil && ctx.Err() == nil {
			slog.Error("cv parse", "run_id", payload.RunID.String(), "err", err.Error())
		}
		return nil
	}
}

// RegisterMetrics dipanggil sekali oleh proses worker CV.
func RegisterMetrics() { registerMetrics() }
