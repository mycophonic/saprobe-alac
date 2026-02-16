#   Copyright Mycophonic.

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.

NAME := saprobe-alac
ALLOWED_LICENSES := Apache-2.0,BSD-3-Clause,MIT
LICENSE_IGNORES := --ignore gotest.tools

include hack/common.mk

# CGO is needed for tests only (CoreAudio reference decoder benchmarks).
# The decoder itself is pure Go — builds must remain CGO_ENABLED=0.
test-unit test-unit-bench test-unit-profile test-unit-cover: export CGO_ENABLED = 1

##########################
# Test reference tools
#
# These are test-only binaries (reference encoders/decoders) used by
# conformance tests and benchmarks.  They live under tests/bin/ so that
# agar.LookFor finds them via the module root, and so that `make clean`
# (which removes bin/) does not destroy them.
##########################

TEST_BIN := $(PROJECT_DIR)/tests/bin

# Apple ALAC reference (Apache-2.0)
ALAC_COMMIT := c38887c5c5e64a4b31108733bd79ca9b2496d987
ALAC_BUILD_DIR := $(TEST_BIN)/tmp/alac

# Build alacconvert from Apple's open-source ALAC reference implementation.
alacconvert: $(TEST_BIN)/alacconvert ## Build Apple ALAC reference converter

$(TEST_BIN)/alacconvert:
	@echo "=== Fetching Apple ALAC ($(ALAC_COMMIT)) ==="
	@rm -rf $(ALAC_BUILD_DIR)
	@mkdir -p $(TEST_BIN)
	@git clone --depth 1 \
		https://github.com/macosforge/alac.git \
		$(ALAC_BUILD_DIR)
	@echo "=== Building alacconvert ==="
	@cd $(ALAC_BUILD_DIR)/convert-utility && $(MAKE)
	@cp $(ALAC_BUILD_DIR)/convert-utility/alacconvert $(TEST_BIN)/alacconvert
	@echo "=== alacconvert built: $(TEST_BIN)/alacconvert ==="

clean-alacconvert: ## Clean ALAC build artifacts
	@rm -rf $(ALAC_BUILD_DIR) $(TEST_BIN)/alacconvert

# CoreAudio ALAC encoder/decoder (macOS)
COREAUDIO_DIR := tests/testutil/coreaudio

# Build alac-coreaudio from CoreAudio (macOS only).
alac-coreaudio: $(TEST_BIN)/alac-coreaudio ## Build CoreAudio ALAC encoder/decoder

$(TEST_BIN)/alac-coreaudio:
	@echo "=== Building alac-coreaudio ==="
	@$(MAKE) -C $(COREAUDIO_DIR) OUTPUT_DIR=$(TEST_BIN)
	@echo "=== alac-coreaudio built: $(TEST_BIN)/alac-coreaudio ==="

clean-coreaudio: ## Clean CoreAudio build artifacts
	@rm -f $(TEST_BIN)/alac-coreaudio
