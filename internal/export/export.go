package export

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jung-kurt/gofpdf"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/xuri/excelize/v2"
)

const (
	FormatJSON  = "json"
	FormatCSV   = "csv"
	FormatExcel = "excel"
	FormatPDF   = "pdf"
)

func ValidFormat(format string) bool {
	switch format {
	case FormatJSON, FormatCSV, FormatExcel, FormatPDF:
		return true
	default:
		return false
	}
}

func ContentType(format string) string {
	switch format {
	case FormatJSON:
		return "application/json"
	case FormatCSV:
		return "text/csv"
	case FormatExcel:
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case FormatPDF:
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func FileExtension(format string) string {
	if format == FormatExcel {
		return "xlsx"
	}
	return format
}

func JSON(job models.Job, reports []models.CandidateReport) ([]byte, error) {
	payload := struct {
		Job     models.Job               `json:"job"`
		Reports []models.CandidateReport `json:"reports"`
	}{Job: job, Reports: reports}

	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling export: %w", err)
	}
	return body, nil
}

var summaryHeader = []string{
	"Rank", "Name", "GitHub Username", "Email", "Overall Score", "Stack Match",
	"Stack Match Score", "Evidence Strength", "Contributions", "LLM Judgment",
	"Status", "Warning", "Report URL",
}

func summaryRow(rank int, r models.CandidateReport) []string {
	return []string{
		strconv.Itoa(rank),
		r.Candidate.Name,
		r.Candidate.GithubUsername,
		r.Candidate.Email,
		formatScore(r.Reasoning.Score),
		r.Reasoning.StackMatch,
		formatScore(r.Reasoning.Breakdown.StackMatch),
		formatScore(r.Reasoning.Breakdown.EvidenceStrength),
		formatScore(r.Reasoning.Breakdown.Contributions),
		formatScore(r.Reasoning.Breakdown.LLMJudgment),
		r.Candidate.Status,
		r.Warning,
		"https://github.com/" + r.Candidate.GithubUsername,
	}
}

var repoHeader = []string{
	"Candidate", "Repo", "Repo URL", "Live URL", "Is Live", "Detected Stack",
	"Commits (90d)", "Active Weeks (90d)", "Has README", "Repo Score", "Release URL",
}

func repoRow(candidateName string, e models.RepoEvidenceSummary) []string {
	return []string{
		candidateName,
		e.Name,
		e.RepoURL,
		e.LiveURL,
		strconv.FormatBool(e.IsLive),
		strings.Join(e.DetectedStack, ", "),
		strconv.Itoa(e.Commits90d),
		strconv.Itoa(e.ActiveWeeks90d),
		strconv.FormatBool(e.HasReadme),
		formatScore(e.Score),
		e.ReleaseURL,
	}
}

var contributionHeader = []string{
	"Candidate", "Repo", "Repo URL", "PR Title", "PR URL", "Merged At",
	"Merged PR Count", "Contributor Count", "Stars",
}

func contributionRow(candidateName string, c models.ContributionSummary) []string {
	return []string{
		candidateName,
		c.RepoOwner + "/" + c.RepoName,
		c.RepoURL,
		c.PRTitle,
		c.PRUrl,
		c.MergedAt,
		strconv.Itoa(c.MergedPRCount),
		strconv.Itoa(c.ContributorCount),
		strconv.Itoa(c.Stars),
	}
}

var stackHeader = []string{"Candidate", "Technology", "Coverage %", "Repo Count", "Repos"}

func stackRow(candidateName string, s models.StackCoverageSummary) []string {
	return []string{
		candidateName,
		s.Technology,
		formatScore(s.Percentage),
		strconv.Itoa(s.RepoCount),
		strings.Join(s.Repos, ", "),
	}
}

var reasonHeader = []string{"Candidate", "Type", "Point", "Evidence Repos"}

func reasonRow(candidateName, kind string, r models.ReasonSummary) []string {
	var projects []string
	for _, e := range r.Evidence {
		projects = append(projects, e.Project)
	}
	return []string{candidateName, kind, r.Point, strings.Join(projects, ", ")}
}

func CSV(job models.Job, reports []models.CandidateReport) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	write := func(row []string) error {
		if err := w.Write(row); err != nil {
			return fmt.Errorf("writing csv row: %w", err)
		}
		return nil
	}

	if err := write([]string{"Job", job.Title}); err != nil {
		return nil, err
	}
	if err := write([]string{"Stack", strings.Join(job.Stack, ", ")}); err != nil {
		return nil, err
	}
	if err := write(nil); err != nil {
		return nil, err
	}

	if err := write([]string{"CANDIDATES"}); err != nil {
		return nil, err
	}
	if err := write(summaryHeader); err != nil {
		return nil, err
	}
	for i, r := range reports {
		if err := write(summaryRow(i+1, r)); err != nil {
			return nil, err
		}
	}
	if err := write(nil); err != nil {
		return nil, err
	}

	if err := write([]string{"REPOS"}); err != nil {
		return nil, err
	}
	if err := write(repoHeader); err != nil {
		return nil, err
	}
	for _, r := range reports {
		for _, e := range r.Evidence {
			if err := write(repoRow(r.Candidate.Name, e)); err != nil {
				return nil, err
			}
		}
	}
	if err := write(nil); err != nil {
		return nil, err
	}

	if err := write([]string{"CONTRIBUTIONS"}); err != nil {
		return nil, err
	}
	if err := write(contributionHeader); err != nil {
		return nil, err
	}
	for _, r := range reports {
		for _, c := range r.Contributions {
			if err := write(contributionRow(r.Candidate.Name, c)); err != nil {
				return nil, err
			}
		}
	}
	if err := write(nil); err != nil {
		return nil, err
	}

	if err := write([]string{"STACK COVERAGE"}); err != nil {
		return nil, err
	}
	if err := write(stackHeader); err != nil {
		return nil, err
	}
	for _, r := range reports {
		for _, s := range r.Reasoning.Breakdown.StackCoverage {
			if err := write(stackRow(r.Candidate.Name, s)); err != nil {
				return nil, err
			}
		}
	}
	if err := write(nil); err != nil {
		return nil, err
	}

	if err := write([]string{"REASONING"}); err != nil {
		return nil, err
	}
	if err := write(reasonHeader); err != nil {
		return nil, err
	}
	for _, r := range reports {
		for _, reason := range r.Reasoning.PositiveReasons {
			if err := write(reasonRow(r.Candidate.Name, "positive", reason)); err != nil {
				return nil, err
			}
		}
		for _, reason := range r.Reasoning.NegativeReasons {
			if err := write(reasonRow(r.Candidate.Name, "negative", reason)); err != nil {
				return nil, err
			}
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("flushing csv: %w", err)
	}
	return buf.Bytes(), nil
}

func Excel(job models.Job, reports []models.CandidateReport) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	writeSheet := func(name string, header []string, rows [][]string) error {
		if name != "Candidates" {
			if _, err := f.NewSheet(name); err != nil {
				return fmt.Errorf("creating sheet %s: %w", name, err)
			}
		}
		for col, h := range header {
			cell, err := excelize.CoordinatesToCellName(col+1, 1)
			if err != nil {
				return fmt.Errorf("computing header cell: %w", err)
			}
			f.SetCellValue(name, cell, h)
		}
		for i, row := range rows {
			for col, v := range row {
				cell, err := excelize.CoordinatesToCellName(col+1, i+2)
				if err != nil {
					return fmt.Errorf("computing cell: %w", err)
				}
				f.SetCellValue(name, cell, v)
			}
		}
		for col := 1; col <= len(header); col++ {
			colName, err := excelize.ColumnNumberToName(col)
			if err != nil {
				return fmt.Errorf("computing column name: %w", err)
			}
			f.SetColWidth(name, colName, colName, 20)
		}
		return nil
	}

	f.SetSheetName("Sheet1", "Candidates")
	f.SetCellValue("Candidates", "A1", "Job")
	f.SetCellValue("Candidates", "B1", job.Title)
	f.SetCellValue("Candidates", "A2", "Stack")
	f.SetCellValue("Candidates", "B2", strings.Join(job.Stack, ", "))
	for col := 1; col <= len(summaryHeader); col++ {
		colName, err := excelize.ColumnNumberToName(col)
		if err != nil {
			return nil, fmt.Errorf("computing column name: %w", err)
		}
		cell, err := excelize.CoordinatesToCellName(col, 4)
		if err != nil {
			return nil, fmt.Errorf("computing header cell: %w", err)
		}
		f.SetCellValue("Candidates", cell, summaryHeader[col-1])
		f.SetColWidth("Candidates", colName, colName, 20)
	}
	for i, r := range reports {
		row := summaryRow(i+1, r)
		for col, v := range row {
			cell, err := excelize.CoordinatesToCellName(col+1, 5+i)
			if err != nil {
				return nil, fmt.Errorf("computing summary cell: %w", err)
			}
			f.SetCellValue("Candidates", cell, v)
		}
	}

	var repoRows, contribRows, stackRows, reasonRows [][]string
	for _, r := range reports {
		for _, e := range r.Evidence {
			repoRows = append(repoRows, repoRow(r.Candidate.Name, e))
		}
		for _, c := range r.Contributions {
			contribRows = append(contribRows, contributionRow(r.Candidate.Name, c))
		}
		for _, s := range r.Reasoning.Breakdown.StackCoverage {
			stackRows = append(stackRows, stackRow(r.Candidate.Name, s))
		}
		for _, reason := range r.Reasoning.PositiveReasons {
			reasonRows = append(reasonRows, reasonRow(r.Candidate.Name, "positive", reason))
		}
		for _, reason := range r.Reasoning.NegativeReasons {
			reasonRows = append(reasonRows, reasonRow(r.Candidate.Name, "negative", reason))
		}
	}

	if err := writeSheet("Repos", repoHeader, repoRows); err != nil {
		return nil, err
	}
	if err := writeSheet("Contributions", contributionHeader, contribRows); err != nil {
		return nil, err
	}
	if err := writeSheet("Stack Coverage", stackHeader, stackRows); err != nil {
		return nil, err
	}
	if err := writeSheet("Reasoning", reasonHeader, reasonRows); err != nil {
		return nil, err
	}

	f.SetActiveSheet(0)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("writing xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

func PDF(job models.Job, reports []models.CandidateReport) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	pdf.AddPage()

	line := func(size float64, style, text string) {
		pdf.SetFont("Helvetica", style, size)
		pdf.MultiCell(0, size/1.8, tr(text), "", "L", false)
	}

	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 10, tr(job.Title), "", 1, "L", false, 0, "")
	line(11, "", "Stack: "+strings.Join(job.Stack, ", "))
	line(11, "", fmt.Sprintf("%d candidates", len(reports)))
	pdf.Ln(4)

	ensureSpace := func(mm float64) {
		if pdf.GetY()+mm > 270 {
			pdf.AddPage()
		}
	}

	for i, r := range reports {
		ensureSpace(20)
		pdf.SetFont("Helvetica", "B", 13)
		pdf.CellFormat(0, 8, tr(fmt.Sprintf("%d. %s (%s)", i+1, r.Candidate.Name, r.Candidate.GithubUsername)), "", 1, "L", false, 0, "")

		line(10, "", fmt.Sprintf("Email: %s | Status: %s | Applied: %s",
			r.Candidate.Email, r.Candidate.Status, r.Candidate.AppliedAt.Format("2006-01-02")))
		line(10, "B", fmt.Sprintf("Overall %.1f — stack %s (%.1f), evidence %.1f, contributions %.1f, LLM %.1f",
			r.Reasoning.Score, r.Reasoning.StackMatch, r.Reasoning.Breakdown.StackMatch,
			r.Reasoning.Breakdown.EvidenceStrength, r.Reasoning.Breakdown.Contributions, r.Reasoning.Breakdown.LLMJudgment,
		))
		if r.Warning != "" {
			line(9, "I", r.Warning)
		}
		pdf.Ln(2)

		if len(r.Reasoning.Breakdown.StackCoverage) > 0 {
			ensureSpace(10)
			line(10, "B", "Stack Coverage")
			for _, s := range r.Reasoning.Breakdown.StackCoverage {
				ensureSpace(6)
				repos := strings.Join(s.Repos, ", ")
				if repos == "" {
					repos = "none"
				}
				line(9, "", fmt.Sprintf("%s: %.0f%% (%d repos — %s)", s.Technology, s.Percentage, s.RepoCount, repos))
			}
			pdf.Ln(2)
		}

		if len(r.Evidence) > 0 {
			ensureSpace(10)
			line(10, "B", "Repos")
			for _, e := range r.Evidence {
				ensureSpace(12)
				stack := strings.Join(e.DetectedStack, ", ")
				if stack == "" {
					stack = "unknown"
				}
				live := "not live"
				if e.IsLive {
					live = "live"
				} else if e.ReleaseURL != "" {
					live = "released " + e.ReleaseTag
				}
				line(9, "", fmt.Sprintf("%s — %s, %s, %d commits/%d active weeks (90d), score %.1f",
					e.Name, stack, live, e.Commits90d, e.ActiveWeeks90d, e.Score))
			}
			pdf.Ln(2)
		}

		if len(r.Contributions) > 0 {
			ensureSpace(10)
			line(10, "B", "External Contributions")
			for _, c := range r.Contributions {
				ensureSpace(6)
				line(9, "", fmt.Sprintf("%s/%s — %q, %d contributors, %d stars",
					c.RepoOwner, c.RepoName, c.PRTitle, c.ContributorCount, c.Stars))
			}
			pdf.Ln(2)
		}

		if len(r.Reasoning.PositiveReasons) > 0 {
			ensureSpace(8)
			line(10, "B", "Positive")
			for _, reason := range r.Reasoning.PositiveReasons {
				ensureSpace(6)
				line(9, "", "+ "+reason.Point)
			}
		}

		if len(r.Reasoning.NegativeReasons) > 0 {
			ensureSpace(8)
			line(10, "B", "Negative")
			for _, reason := range r.Reasoning.NegativeReasons {
				ensureSpace(6)
				line(9, "", "- "+reason.Point)
			}
		}

		pdf.Ln(6)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("writing pdf: %w", err)
	}
	return buf.Bytes(), nil
}

func Build(format string, job models.Job, reports []models.CandidateReport) ([]byte, error) {
	switch format {
	case FormatJSON:
		return JSON(job, reports)
	case FormatCSV:
		return CSV(job, reports)
	case FormatExcel:
		return Excel(job, reports)
	case FormatPDF:
		return PDF(job, reports)
	default:
		return nil, fmt.Errorf("unsupported export format %q", format)
	}
}

func formatScore(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64)
}
