package handler

import (
	"bytes"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"

	"nvims/internal/store"
)

// generateTCCPDF produces a PDF snapshot of the teacher's TCCP for one faculty.
func generateTCCPDF(teacherName string, year int, facultyName, vccStatus string, programs []store.ProgramWithSubjects) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(18, 18, 18)
	pdf.SetAutoPageBreak(true, 18)

	// ── Colour helpers ────────────────────────────────────────────────────────
	setFill := func(r, g, b int) { pdf.SetFillColor(r, g, b) }
	setDraw := func(r, g, b int) { pdf.SetDrawColor(r, g, b) }
	setText := func(r, g, b int) { pdf.SetTextColor(r, g, b) }

	// ── Header page ───────────────────────────────────────────────────────────
	pdf.AddPage()

	// Navy bar at top
	setFill(30, 41, 59)
	pdf.Rect(0, 0, 210, 22, "F")
	pdf.SetFont("Helvetica", "B", 13)
	setText(255, 255, 255)
	pdf.SetXY(18, 6)
	pdf.Cell(0, 10, "NVIMS — Trainer Competency & Currency Profile")

	// Reset text colour
	setText(30, 41, 59)

	// Main title block
	pdf.SetXY(18, 30)
	pdf.SetFont("Helvetica", "B", 18)
	setText(30, 41, 59)
	pdf.Cell(0, 10, "Trainer Competency &")
	pdf.SetXY(18, 40)
	pdf.Cell(0, 10, "Currency Profile (TCCP)")

	pdf.SetXY(18, 55)
	pdf.SetFont("Helvetica", "", 11)
	setText(71, 85, 105)

	labelCol := func(label, value string) {
		pdf.SetFont("Helvetica", "B", 10)
		setText(71, 85, 105)
		pdf.Cell(40, 7, label+":")
		pdf.SetFont("Helvetica", "", 10)
		setText(15, 23, 42)
		pdf.Cell(0, 7, value)
		pdf.Ln(7)
	}

	pdf.SetX(18)
	labelCol("Teacher", teacherName)
	pdf.SetX(18)
	labelCol("Faculty", facultyName)
	pdf.SetX(18)
	labelCol("Calendar Year", fmt.Sprintf("%d", year))
	pdf.SetX(18)
	labelCol("VCC Status", vccStatus)
	pdf.SetX(18)
	labelCol("Published", time.Now().Format("2 January 2006"))

	// Legend box
	pdf.SetXY(18, 95)
	setFill(241, 245, 249)
	setDraw(226, 232, 240)
	pdf.RoundedRect(18, 95, 174, 32, 3, "1234", "FD")
	pdf.SetXY(22, 99)
	pdf.SetFont("Helvetica", "B", 9)
	setText(71, 85, 105)
	pdf.Cell(0, 6, "LEGEND")
	pdf.SetXY(22, 106)
	pdf.SetFont("Helvetica", "", 9)
	setText(15, 23, 42)
	pdf.Cell(16, 5, "[OK]")
	pdf.Cell(0, 5, "Evidence linked and Approved")
	pdf.SetXY(22, 112)
	pdf.Cell(16, 5, "[~]")
	pdf.Cell(0, 5, "Evidence linked, awaiting approval (Draft/Pending)")
	pdf.SetXY(22, 118)
	pdf.Cell(16, 5, "[-]")
	pdf.Cell(0, 5, "No evidence linked")
	pdf.SetXY(100, 106)
	pdf.Cell(8, 5, "P")
	pdf.Cell(0, 5, "= VET Currency (Professional Evidence)")
	pdf.SetXY(100, 112)
	pdf.Cell(8, 5, "V")
	pdf.Cell(0, 5, "= Vocational Competency")
	pdf.SetXY(100, 118)
	pdf.Cell(8, 5, "I")
	pdf.Cell(0, 5, "= Industry Currency")

	// Divider
	setDraw(226, 232, 240)
	pdf.SetLineWidth(0.3)
	pdf.Line(18, 133, 192, 133)

	// ── Programs ──────────────────────────────────────────────────────────────
	colW := [5]float64{30, 100, 14, 14, 14} // Code, Name, P, V, I
	hdrH := float64(7)
	rowH := float64(6)

	drawTableHeader := func() {
		setFill(30, 41, 59)
		setDraw(30, 41, 59)
		setText(255, 255, 255)
		pdf.SetFont("Helvetica", "B", 8)
		pdf.CellFormat(colW[0], hdrH, "Code", "1", 0, "L", true, 0, "")
		pdf.CellFormat(colW[1], hdrH, "Subject", "1", 0, "L", true, 0, "")
		pdf.CellFormat(colW[2], hdrH, "P", "1", 0, "C", true, 0, "")
		pdf.CellFormat(colW[3], hdrH, "V", "1", 0, "C", true, 0, "")
		pdf.CellFormat(colW[4], hdrH, "I", "1", 1, "C", true, 0, "")
		setText(15, 23, 42)
	}

	iconText := func(hasAny, hasApproved bool) string {
		if !hasAny {
			return "[-]"
		}
		if hasApproved {
			return "[OK]"
		}
		return "[~]"
	}

	pdf.SetXY(18, 138)

	for _, prog := range programs {
		// Check if we have room for the program header + at least one row
		if pdf.GetY() > 255 {
			pdf.AddPage()
		}

		// Program header band
		setFill(241, 245, 249)
		setDraw(226, 232, 240)
		setText(30, 41, 59)
		pdf.SetFont("Helvetica", "B", 9)
		pdf.CellFormat(174, 8, fmt.Sprintf("%s  —  %s", prog.ProgramCode, prog.ProgramName), "1", 1, "L", true, 0, "")

		if len(prog.Subjects) == 0 {
			pdf.SetFont("Helvetica", "I", 8)
			setText(148, 163, 184)
			pdf.CellFormat(174, 6, "No subjects.", "LRB", 1, "L", false, 0, "")
			setText(15, 23, 42)
			pdf.Ln(2)
			continue
		}

		drawTableHeader()

		alt := false
		for _, s := range prog.Subjects {
			if pdf.GetY() > 270 {
				pdf.AddPage()
				drawTableHeader()
				alt = false
			}
			if alt {
				setFill(248, 250, 252)
			} else {
				setFill(255, 255, 255)
			}
			alt = !alt

			setDraw(226, 232, 240)
			pdf.SetFont("Helvetica", "", 7)
			setText(15, 23, 42)
			pdf.CellFormat(colW[0], rowH, s.SubjectCode, "1", 0, "L", true, 0, "")

			// Truncate long subject names
			name := s.SubjectName
			if len(name) > 60 {
				name = name[:57] + "..."
			}
			pdf.CellFormat(colW[1], rowH, name, "1", 0, "L", true, 0, "")

			// P column
			pText := iconText(s.HasProf, s.HasProfApproved)
			if s.HasProfApproved {
				setText(22, 163, 74)
			} else if s.HasProf {
				setText(217, 119, 6)
			} else {
				setText(148, 163, 184)
			}
			pdf.SetFont("Helvetica", "B", 7)
			pdf.CellFormat(colW[2], rowH, pText, "1", 0, "C", true, 0, "")

			// V column
			vText := iconText(s.HasVoc, s.HasVocApproved)
			if s.HasVocApproved {
				setText(22, 163, 74)
			} else if s.HasVoc {
				setText(217, 119, 6)
			} else {
				setText(148, 163, 184)
			}
			pdf.CellFormat(colW[3], rowH, vText, "1", 0, "C", true, 0, "")

			// I column
			iText := iconText(s.HasInd, s.HasIndApproved)
			if s.HasIndApproved {
				setText(22, 163, 74)
			} else if s.HasInd {
				setText(217, 119, 6)
			} else {
				setText(148, 163, 184)
			}
			pdf.CellFormat(colW[4], rowH, iText, "1", 1, "C", true, 0, "")

			setText(15, 23, 42)
		}
		pdf.Ln(3)
	}

	// ── Footer on every page ─────────────────────────────────────────────────
	pdf.SetFooterFunc(func() {
		pdf.SetY(-14)
		pdf.SetFont("Helvetica", "I", 8)
		setText(148, 163, 184)
		pdf.CellFormat(0, 5,
			fmt.Sprintf("TCCP — %s — %s — Page %d / {nb}", teacherName, time.Now().Format("2006-01-02"), pdf.PageNo()),
			"", 0, "C", false, 0, "")
	})
	pdf.AliasNbPages("{nb}")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
