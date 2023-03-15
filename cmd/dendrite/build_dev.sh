set -xe
if [ -z "$(git status --porcelain)" ]; then 
    CGO_ENABLED=0 go build .
    TAG=$(git rev-parse --short HEAD)
    docker build -f Dockerfile.dev -t gcr.io/globekeeper-development/dendrite-monolith:$TAG -t gcr.io/globekeeper-development/dendrite-monolith  -t gcr.io/globekeeper-production/dendrite-monolith:$TAG .
    docker push gcr.io/globekeeper-development/dendrite-monolith:$TAG
    docker push gcr.io/globekeeper-production/dendrite-monolith:$TAG
    docker push gcr.io/globekeeper-development/dendrite-monolith
else 
    echo "Please commit changes"
    exit 0
fi