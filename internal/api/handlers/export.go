package handlers

import (
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/api/middleware"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/export"
	"github.com/var-raphael/groundtruth/internal/plans"
)

type ExportHandler struct {
	Pool *pgxpool.Pool
}

func (h *ExportHandler) ExportJob(w http.ResponseWriter, r *http.Request) {
	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	jobID := r.PathValue("id")
	job, err := queries.GetJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	if job.RecruiterID != recruiterID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = export.FormatJSON
	}
	if !export.ValidFormat(format) {
		http.Error(w, "format must be one of: json, csv, excel, pdf", http.StatusBadRequest)
		return
	}

	recruiter, err := queries.GetRecruiterByID(r.Context(), h.Pool, recruiterID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if recruiter == nil {
		http.Error(w, "recruiter not found", http.StatusUnauthorized)
		return
	}
	if !plans.For(recruiter.Plan).AllowsExport(format) {
		http.Error(w, "your plan does not include this export format, upgrade to continue", http.StatusForbidden)
		return
	}

	reports, err := queries.AllReportsByJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body, err := export.Build(format, *job, reports)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := queries.MarkJobDeleteConfirmationDownloaded(r.Context(), h.Pool, jobID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("%s-report.%s", job.ID, export.FileExtension(format))
	w.Header().Set("Content-Type", export.ContentType(format))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}
