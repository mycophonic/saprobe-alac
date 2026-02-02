#   Copyright Mycophonic.

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.NAME := alac
ALLOWED_LICENSES := Apache-2.0,BSD-2-Clause,BSD-3-Clause,MIT
LICENSE_IGNORES := --ignore gotest.tools

include hack/common.mk

##########################
# Apple ALAC reference (Apache-2.0)
##########################

ALAC_COMMIT := c38887c5c5e64a4b31108733bd79ca9b2496d987
ALAC_BUILD_DIR := bin/tmp/alac

# Build alacconvert from Apple's open-source ALAC reference implementation.
alacconvert: bin/alacconvert ## Build Apple ALAC reference converter

bin/alacconvert:
	@echo "=== Fetching Apple ALAC ($(ALAC_COMMIT)) ==="
	@rm -rf $(ALAC_BUILD_DIR)
	@mkdir -p bin
	@git clone --depth 1 \
		https://github.com/macosforge/alac.git \
		$(ALAC_BUILD_DIR)
	@echo "=== Building alacconvert ==="
	@cd $(ALAC_BUILD_DIR)/convert-utility && $(MAKE)
	@cp $(ALAC_BUILD_DIR)/convert-utility/alacconvert bin/alacconvert
	@echo "=== alacconvert built: bin/alacconvert ==="

clean-alacconvert: ## Clean ALAC build artifacts
	@rm -rf $(ALAC_BUILD_DIR) bin/alacconvert

##########################
# CoreAudio ALAC encoder/decoder (macOS)
##########################

COREAUDIO_DIR := tests/testutil/coreaudio

# Build alac-coreaudio from CoreAudio (macOS only).
alac-coreaudio: bin/alac-coreaudio ## Build CoreAudio ALAC encoder/decoder

bin/alac-coreaudio:
	@echo "=== Building alac-coreaudio ==="
	@$(MAKE) -C $(COREAUDIO_DIR) OUTPUT_DIR=$(CURDIR)/bin
	@echo "=== alac-coreaudio built: bin/alac-coreaudio ==="

clean-coreaudio: ## Clean CoreAudio build artifacts
	@rm -f bin/alac-coreaudio
