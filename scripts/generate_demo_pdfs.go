package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jung-kurt/gofpdf/v2"
)

func main() {
	outDir := filepath.Join("demo", "income_verification")
	os.MkdirAll(outDir, 0755)

	createPaystub(filepath.Join(outDir, "1_Paystub_2025.pdf"))
	createW2(filepath.Join(outDir, "2_W2_Form_2025.pdf"))
	create1040(filepath.Join(outDir, "3_Form1040_TaxReturn_2025.pdf"))
	createBankStatement(filepath.Join(outDir, "4_BankStatement_Q4_2025.pdf"))
	fmt.Println("All PDFs generated successfully")
}

func newPDF() *gofpdf.Fpdf {
	pdf := gofpdf.New("P", "mm", "Letter", "")
	pdf.SetMargins(18, 18, 18)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)
	return pdf
}

func createPaystub(filename string) {
	pdf := newPDF()

	pdf.SetFont("Helvetica", "B", 14)
	pdf.Cell(0, 8, "ACME GLOBAL ENTERPRISES LLC")
	pdf.Ln(8)
	pdf.SetFont("Helvetica", "", 8)
	pdf.Cell(0, 5, "100 Corporate Parkway, Suite 400, New York, NY 10001")
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 8, "EARNINGS STATEMENT / PAYSTUB")
	pdf.Ln(12)

	pdf.SetFont("Helvetica", "", 9)
	type kv struct{ label, value string }
	for _, i := range []kv{
		{"Employee Name:", "Johnathan Doe"},
		{"Employee ID:", "EMP-884920"},
		{"SSN (Tax ID):", "XXX-XX-6789"},
		{"Pay Period:", "11/16/2025 - 11/30/2025"},
		{"Pay Date:", "12/01/2025"},
		{"Pay Frequency:", "Semi-Monthly (24/yr)"},
	} {
		pdf.SetFont("Helvetica", "B", 9)
		pdf.Cell(35, 5, i.label)
		pdf.SetFont("Helvetica", "", 9)
		pdf.Cell(50, 5, i.value)
		pdf.Ln(5)
	}
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(26, 54, 93)
	pdf.SetTextColor(255, 255, 255)
	widths := []float64{65, 20, 25, 35, 35}
	for i, h := range []string{"Earnings Description", "Hours", "Rate", "Current", "YTD"} {
		pdf.CellFormat(widths[i], 7, h, "1", 0, "R", true, 0, "")
	}
	pdf.Ln(7)

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 9)
	type row struct{ desc, hours, rate, current, ytd string }
	for _, r := range []row{
		{"Regular Base Salary", "86.67", "$57.69", "$5,000.00", "$120,000.00"},
		{"Discretionary Q3 Executive Incentive", "\u2014", "\u2014", "$0.00", "$18,000.00"},
	} {
		pdf.CellFormat(widths[0], 6, r.desc, "1", 0, "L", false, 0, "")
		pdf.CellFormat(widths[1], 6, r.hours, "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[2], 6, r.rate, "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[3], 6, r.current, "1", 0, "R", false, 0, "")
		pdf.CellFormat(widths[4], 6, r.ytd, "1", 0, "R", false, 0, "")
		pdf.Ln(6)
	}

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(237, 242, 247)
	for i, v := range []string{"GROSS EARNINGS", "", "", "$5,000.00", "$138,000.00"} {
		pdf.CellFormat(widths[i], 6, v, "1", 0, "R", true, 0, "")
	}
	pdf.Ln(12)

	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(43, 108, 176)
	pdf.Cell(40, 6, "NET PAY:")
	pdf.SetTextColor(0, 0, 0)
	pdf.Cell(30, 6, "$3,541.66")
	pdf.SetTextColor(197, 48, 48)
	pdf.Cell(40, 6, "ANNUALIZED YTD GROSS:")
	pdf.Cell(30, 6, "$138,000.00")

	pdf.OutputFileAndClose(filename)
	fmt.Printf("Created %s\n", filename)
}

func createW2(filename string) {
	pdf := newPDF()

	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 8, "Form W-2 Wage and Tax Statement 2025")
	pdf.Ln(8)
	pdf.SetFont("Helvetica", "", 8)
	pdf.Cell(0, 5, "Department of the Treasury \u2014 Internal Revenue Service")
	pdf.Ln(12)

	type cell struct{ label, value string }
	cells := []cell{
		{"a Employee's social security number", "XXX-XX-6789"},
		{"1 Wages, tips, other compensation", "$120,000.00"},
		{"2 Federal income tax withheld", "$21,000.00"},
		{"b Employer identification number (EIN)", "12-3456789"},
		{"3 Social security wages", "$120,000.00"},
		{"4 Social security tax withheld", "$7,440.00"},
		{"c Employer's name, address, and ZIP code", "ACME Global Enterprises LLC"},
		{"5 Medicare wages and tips", "$120,000.00"},
		{"6 Medicare tax withheld", "$1,740.00"},
		{"e Employee's name and address", "Johnathan Doe"},
		{"15 State / Employer's state ID", "NY / 9918273"},
		{"16 State wages, tips, etc.", "$120,000.00"},
	}

	colW := []float64{60, 55, 55}
	for i := 0; i < len(cells); i += 3 {
		y := pdf.GetY()
		for j := 0; j < 3 && i+j < len(cells); j++ {
			idx := i + j
			x := 18 + colW[0]*float64(j)
			pdf.SetFont("Helvetica", "B", 7)
			pdf.SetXY(x, y)
			pdf.MultiCell(colW[j], 3.5, cells[idx].label, "LT", "L", false)
			pdf.SetFont("Helvetica", "", 8)
			pdf.SetXY(x, pdf.GetY())
			pdf.MultiCell(colW[j], 4, cells[idx].value, "LB", "L", false)
		}
		pdf.SetY(y + 18)
	}

	pdf.OutputFileAndClose(filename)
	fmt.Printf("Created %s\n", filename)
}

func create1040(filename string) {
	pdf := newPDF()

	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 8, "Form 1040 - U.S. Individual Income Tax Return 2025")
	pdf.Ln(12)

	pdf.SetFont("Helvetica", "", 9)
	type kv struct{ label, value string }
	for _, i := range []kv{
		{"Your First Name and Middle Initial", "Johnathan"},
		{"Last Name", "Doe"},
		{"Home Address", "742 Evergreen Terrace"},
		{"City, State, ZIP", "Springfield, OR 97477"},
		{"Filing Status:", "[X] Single"},
		{"SSN:", "XXX-XX-6789"},
	} {
		pdf.SetFont("Helvetica", "B", 9)
		pdf.Cell(50, 5, i.label)
		pdf.SetFont("Helvetica", "", 9)
		pdf.Cell(60, 5, i.value)
		pdf.Ln(5)
	}
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "B", 10)
	pdf.Cell(0, 7, "Income Lines")
	pdf.Ln(8)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(254, 252, 191)
	pdf.CellFormat(15, 6, "Line", "1", 0, "C", true, 0, "")
	pdf.CellFormat(90, 6, "Description", "1", 0, "L", true, 0, "")
	pdf.CellFormat(40, 6, "Amount", "1", 0, "R", true, 0, "")
	pdf.Ln(6)

	type line struct{ ln, desc, amt string }
	for _, l := range []line{
		{"1a", "Total amount from Form(s) W-2, box 1", "$120,000.00"},
		{"1z", "Add lines 1a through 1h (Total Wages)", "$120,000.00"},
		{"2b", "Taxable interest", "$450.00"},
		{"9", "Total Income", "$120,450.00"},
		{"11", "Adjusted Gross Income (AGI)", "$120,450.00"},
		{"15", "Taxable Income", "$105,850.00"},
	} {
		bold := l.ln == "1z"
		if bold {
			pdf.SetFont("Helvetica", "B", 9)
			pdf.SetFillColor(254, 252, 191)
		}
		pdf.CellFormat(15, 6, l.ln, "1", 0, "C", bold, 0, "")
		pdf.CellFormat(90, 6, l.desc, "1", 0, "L", bold, 0, "")
		pdf.CellFormat(40, 6, l.amt, "1", 0, "R", bold, 0, "")
		pdf.Ln(6)
		if bold {
			pdf.SetFont("Helvetica", "", 9)
		}
	}

	pdf.OutputFileAndClose(filename)
	fmt.Printf("Created %s\n", filename)
}

func createBankStatement(filename string) {
	pdf := newPDF()

	pdf.SetFont("Helvetica", "B", 14)
	pdf.Cell(0, 8, "FIRST NATIONAL HORIZON BANK")
	pdf.Ln(8)
	pdf.SetFont("Helvetica", "", 8)
	pdf.Cell(0, 5, "ACCOUNT STATEMENT | Statement Period: 10/01/2025 to 12/31/2025")
	pdf.Ln(12)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.Cell(35, 5, "Account Holder:")
	pdf.SetFont("Helvetica", "", 9)
	pdf.Cell(50, 5, "Johnathan Doe")
	pdf.SetFont("Helvetica", "B", 9)
	pdf.Cell(35, 5, "Account Number:")
	pdf.SetFont("Helvetica", "", 9)
	pdf.Cell(50, 5, "XXXX-XXXX-4819")
	pdf.Ln(8)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.Cell(35, 5, "Branch:")
	pdf.SetFont("Helvetica", "", 9)
	pdf.Cell(50, 5, "Springfield Main St")
	pdf.SetFont("Helvetica", "B", 9)
	pdf.Cell(35, 5, "Account Type:")
	pdf.SetFont("Helvetica", "", 9)
	pdf.Cell(50, 5, "Premier Checking")
	pdf.Ln(12)

	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(43, 108, 176)
	pdf.SetTextColor(255, 255, 255)
	sumWidths := []float64{42, 42, 42, 42}
	for i, h := range []string{"Starting Bal", "Total Deposits", "Total Withdrawals", "Ending Bal"} {
		pdf.CellFormat(sumWidths[i], 7, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(7)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 9)
	for i, v := range []string{"$14,210.00", "$21,250.00", "($14,150.00)", "$21,310.00"} {
		pdf.CellFormat(sumWidths[i], 6, v, "1", 0, "C", false, 0, "")
	}
	pdf.Ln(12)

	pdf.SetFont("Helvetica", "B", 10)
	pdf.Cell(0, 7, "Recurring Payroll Deposit Activity (Q4 2025)")
	pdf.Ln(8)

	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(237, 242, 247)
	txWidths := []float64{22, 78, 20, 30, 30}
	for i, h := range []string{"Date", "Description", "Type", "Amount", "Balance"} {
		pdf.CellFormat(txWidths[i], 6, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(6)

	pdf.SetFont("Helvetica", "", 8)
	type tx struct{ date, desc, typ, amt, bal string }
	for _, t := range []tx{
		{"10/01", "ACH Direct Deposit - ACME GLOBAL PAYROLL", "Credit", "+$3,541.66", "$17,751.66"},
		{"10/16", "ACH Direct Deposit - ACME GLOBAL PAYROLL", "Credit", "+$3,541.66", "$21,293.32"},
		{"11/01", "ACH Direct Deposit - ACME GLOBAL PAYROLL", "Credit", "+$3,541.66", "$24,834.98"},
		{"11/16", "ACH Direct Deposit - ACME GLOBAL PAYROLL", "Credit", "+$3,541.66", "$28,376.64"},
		{"12/01", "ACH Direct Deposit - ACME GLOBAL PAYROLL", "Credit", "+$3,541.66", "$31,918.30"},
		{"12/16", "ACH Direct Deposit - ACME GLOBAL PAYROLL", "Credit", "+$3,541.66", "$21,310.00"},
	} {
		pdf.CellFormat(txWidths[0], 5, t.date, "1", 0, "L", false, 0, "")
		pdf.CellFormat(txWidths[1], 5, t.desc, "1", 0, "L", false, 0, "")
		pdf.CellFormat(txWidths[2], 5, t.typ, "1", 0, "C", false, 0, "")
		pdf.CellFormat(txWidths[3], 5, t.amt, "1", 0, "R", false, 0, "")
		pdf.CellFormat(txWidths[4], 5, t.bal, "1", 0, "R", false, 0, "")
		pdf.Ln(5)
	}
	pdf.Ln(5)

	pdf.SetFont("Helvetica", "I", 7)
	pdf.MultiCell(0, 4, "Note: Total 3-month net payroll deposits = $21,250 ($85,000 annualized net take-home cash flow). Represents post-tax net liquidity.", "", "L", false)

	pdf.OutputFileAndClose(filename)
	fmt.Printf("Created %s\n", filename)
}
