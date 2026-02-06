#!/bin/bash
set -e

# Pick up PROJECT_ID from env variable, but default to deploy-demo-test
PROJECT_ID=${PROJECT_ID:-deploy-demo-test}
REGION=${REGION:-us-central1}
SERVICE_NAME=${SERVICE_NAME:-scion-docs}

echo "Deploying Scion Documentation Site..."
echo "Project ID: $PROJECT_ID"
echo "Region:     $REGION"
echo "Service:    $SERVICE_NAME"

# Get the directory where the script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
# Go to the repo root to run the build
cd "$SCRIPT_DIR/.."

# Submit to Cloud Build
# We pass the project explicitly to gcloud
gcloud builds submit 
  --project "$PROJECT_ID" 
  --config docs-site/cloudbuild.yaml 
  --substitutions="_SERVICE_NAME=$SERVICE_NAME,_REGION=$REGION" 
  .
