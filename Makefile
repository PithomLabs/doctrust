.PHONY: build run test ingest clean setup demo-pdfs check-policy test-policy validate-fixtures eval compile-policy test-compiler server regression promote registry

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

# Clean build artifacts
clean:
	rm -rf bin/ compiled/
	rm -f demo/income_verification/evidence_snapshot.json
	rm -f rulesets/*/working.yaml

# Setup: download dependencies
setup:
	go mod tidy
