.PHONY: build run test ingest clean setup demo-pdfs check-policy test-policy validate-fixtures eval compile-policy test-compiler server regression promote registry mcp author-check review-check promote-check verify-ruleset verify-audit resign-publication

# Build all binaries
build:
	go build -o bin/ingest ./cmd/ingest/
	go build -o bin/eval ./cmd/eval/
	go build -o bin/validate-fixtures ./cmd/validate-fixtures/
	go build -o bin/compile-policy ./cmd/compile-policy/
	go build -o bin/server ./cmd/server/
	go build -o bin/regression ./cmd/regression/
	go build -o bin/promote ./cmd/promote/
	go build -o bin/registry ./cmd/registry/
	go build -o bin/author-check ./cmd/author-check/
	go build -o bin/review-check ./cmd/review-check/
	go build -o bin/promote-check ./cmd/promote-check/
	go build -o bin/verify-ruleset ./cmd/verify-ruleset/
	go build -o bin/evidence-mcp ./cmd/evidence-mcp/
	go build -o bin/doctrust-mcp ./cmd/doctrust-mcp/
	go build -o bin/doctrust-review ./cmd/doctrust-review/
	go build -o bin/verify-audit ./cmd/verify-audit/
	go build -o bin/resign-publication ./cmd/resign-publication/

# Run the ingest pipeline on demo documents
run: build
	bin/ingest demo/income_verification/

# Run Go tests
test:
	go test ./...

# Generate demo PDFs
demo-pdfs:
	go run scripts/generate_demo_pdfs.go

# Validate Rego policy syntax
check-policy:
	opa check policies/income_verification/policy.rego

# Run OPA unit tests
test-policy:
	opa test policies/income_verification/policy.rego policies/income_verification/policy_test.rego -v

# Run independent fixture validation (10 fixtures: 7 original + 3 adversarial)
validate-fixtures: build
	bin/validate-fixtures .

# Evaluate policy against a snapshot
eval: build
	bin/eval --policy policies/income_verification/policy.rego demo/income_verification/evidence_snapshot.json

# Compile policy from POLICY.md
compile-policy: build
	bin/compile-policy policies/income_verification/POLICY.md

# Run compiler unit tests
test-compiler:
	go test ./internal/compiler/ -v

# Run the demo web server
server: build
	bin/server --dir demo/income_verification --policy policies/income_verification/policy.rego

# Run regression: compare candidate ruleset against current promoted
regression: build
	bin/regression --domain income_verification

# Run regression with explicit draft
regression-draft: build
	bin/regression --domain income_verification --draft rulesets/income_verification/working.yaml

# Run regression in legacy mode (scenario-level params)
regression-legacy: build
	bin/regression --domain income_verification --scenario-params

# Promote working draft to next immutable version
promote: build
	bin/promote --domain income_verification

# Dry-run promotion (validate only)
promote-dry: build
	bin/promote --domain income_verification --dry-run

# Show registry state
registry: build
	bin/registry

# Show registry for specific domain
registry-domain: build
	bin/registry --domain income_verification

# === Phase 3: candidate check lifecycle ===

# Review a candidate interactively (approve/reject/edit)
review-candidate: build
	bin/review-check $(CANDIDATE)

# Full promotion pipeline for an approved candidate (all trust gates)
promote-candidate: build
	bin/promote-check --candidate $(CANDIDATE) --domain $(DOMAIN)

# Verify promoted ruleset integrity
verify-ruleset: build
	bin/verify-ruleset --domain $(DOMAIN)

# Build MCP stdio server
mcp: build
	go build -o bin/doctrust-mcp ./cmd/doctrust-mcp/

# Clean build artifacts
clean:
	rm -rf bin/ compiled/
	rm -f demo/income_verification/evidence_snapshot.json
	rm -f rulesets/*/working.yaml

# Setup: download dependencies, validate env, build all binaries
setup:
	@echo "== setup: go mod tidy =="
	go mod tidy
	@echo "== setup: validating required env =="
	@if [ -z "$$NUTRIENT_DWS_EXTRACTION_API_KEY" ] && ! grep -q "NUTRIENT_DWS_EXTRACTION_API_KEY=" .env 2>/dev/null; then echo "FAIL: NUTRIENT_DWS_EXTRACTION_API_KEY not set (set in env or .env)"; exit 1; fi
	@echo "env OK (value not printed)"
	@echo "== setup: building all binaries =="
	$(MAKE) build
	@echo "== setup: verifying MCP boundary =="
	$(MAKE) lint-imports
	@echo "setup complete"

# One-command judge demos (Phase A)
demo-pass:
	@echo "== demo-pass: coherent PASS package =="
	./scripts/demo-pass.sh

demo-review:
	@echo "== demo-review: anomaly package → REVIEW → human authority =="
	./scripts/demo-review.sh

verify-audit:
	@if [ -z "$(SNAPSHOT_PATH)" ]; then echo "FAIL: SNAPSHOT_PATH required — usage: make verify-audit SNAPSHOT_PATH=/path/to/evidence_snapshot.json"; exit 1; fi; \
	bin/verify-audit "$(SNAPSHOT_PATH)"

# Authoritative provider-boundary check
# internal/service and cmd/doctrust-mcp must NOT depend on
# internal/nutrient, internal/extraction, or internal/opa
lint-imports:
	@echo "Checking service boundary..."
	@if go list -deps ./internal/service 2>/dev/null | grep -qE "internal/(nutrient|extraction|opa)"; then \
		echo "FAIL: internal/service depends on forbidden package"; \
		go list -deps ./internal/service | grep -E "internal/(nutrient|extraction|opa)"; \
		exit 1; \
	fi
	@if [ -d cmd/doctrust-mcp ]; then \
		echo "Checking MCP boundary..."; \
		if go list -deps ./cmd/doctrust-mcp 2>/dev/null | grep -qE "internal/(nutrient|extraction|opa)"; then \
			echo "FAIL: cmd/doctrust-mcp depends on forbidden package"; \
			exit 1; \
		fi; \
	fi
	@echo "PASS: provider boundary clean"
