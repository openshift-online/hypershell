VENV    = .venv
PYTHON  = python3
BIN     = $(VENV)/bin/build-pipeline

# Runtime knobs (override on the command line or via environment)
PROFILE  ?= tiered
MODE     ?= supervised
RUN_ARGS ?=

# ============================================================================
# Install (idempotent: only re-runs if pyproject.toml changes)
# ============================================================================

.PHONY: install
install: $(BIN)

$(BIN): pyproject.toml
	$(PYTHON) -m venv $(VENV)
	$(VENV)/bin/pip install -e ".[dev,anthropic]" --quiet
	@touch $(BIN)

# ============================================================================
# Common operations
# ============================================================================

.PHONY: test
test: $(BIN)
	$(VENV)/bin/python -m unittest discover -s tests

.PHONY: list
list: $(BIN)
	$(BIN) list

.PHONY: profiles
profiles: $(BIN)
	$(BIN) profiles

.PHONY: dry-run
dry-run: $(BIN)
	$(BIN) run --profile $(PROFILE) --dry-run

.PHONY: run
run: $(BIN)
	$(BIN) run --profile $(PROFILE) --mode $(MODE) $(RUN_ARGS)

.PHONY: runs
runs: $(BIN)
	$(BIN) runs

.PHONY: check-drift
check-drift: $(BIN)
	$(BIN) check-drift

.PHONY: clean
clean:
	rm -rf $(VENV) hypershell_build_pipeline.egg-info .build-pipeline
