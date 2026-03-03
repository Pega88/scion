#!/bin/bash
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# hack/gce-demo-deploy.sh - One-stop deployment for the Scion Demo Hub

set -euo pipefail

echo "=== Scion Demo Full Deployment ==="

# Step 1: Provision Infrastructure
echo ""
echo "--- Step 1: Provisioning Infrastructure ---"
./hack/gce-demo-provision.sh

# Step 2: Setup Repository
echo ""
echo "--- Step 2: Setting up Repository ---"
./hack/gce-demo-setup-repo.sh

# Step 3: Build and Start Hub
echo ""
echo "--- Step 3: Building and Starting Hub ---"
./hack/gce-start-hub.sh --full

echo ""
echo "=== Full Deployment Complete ==="
echo "Your Scion Hub should now be available at https://hub.demo.scion-ai.dev"
