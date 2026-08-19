# Income Verification Policy

## Required Evidence

- paystub
- w2
- form_1040

## Extraction Schema

- paystub: annualized_gross_ytd, base_salary_ytd, bonus_ytd
- w2: wages_tips_other_compensation
- form_1040: line1z_wages
- bank_statement: total_deposits

## Semantic Classification

- paystub.annualized_gross_ytd → gross_income_projected
- paystub.base_salary_ytd → base_salary
- paystub.bonus_ytd → bonus_compensation
- w2.wages_tips_other_compensation → gross_income_taxable
- form_1040.line1z_wages → gross_income_taxable
- bank_statement.total_deposits → net_cash_flow

## Rules

- w2.wages_tips_other_compensation must equal form_1040.line1z_wages
- paystub.annualized_gross_ytd variance over 5% requires human review
- confidence below 0.8 requires human review
- net_cash_flow is incomparable to gross_income_taxable

## Decisions

- PASS when all required evidence is present and no review/violation exists
- REVIEW when evidence is ambiguous or requires human verification
- FAIL when a mandatory rule is violated
- MISSING_EVIDENCE when required evidence is absent
