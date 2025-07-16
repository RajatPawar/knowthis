.PHONY: test build deploy help

# Default target
help:
	@echo "Available commands:"
	@echo "  make test    - Run all tests"
	@echo "  make build   - Build the application"
	@echo "  make deploy  - Run tests, build, and deploy (git push)"
	@echo "  make help    - Show this help message"

# Run tests
test:
	@echo "Running tests..."
	go test ./... -short
	@echo "✅ Tests passed!"

# Build the application
build: test
	@echo "Building application..."
	go build -o knowthis main.go
	@echo "✅ Build successful!"

# Deploy: run tests, build, and push to git
deploy: test build
	@echo "🚀 Deploying..."
	@echo "Adding changes to git..."
	git add .
	@echo "Committing changes..."
	git commit -m "Deploy: $(shell date '+%Y-%m-%d %H:%M:%S') - Auto-deploy after tests passed" || echo "No changes to commit"
	@echo "Pushing to remote..."
	git push
	@echo "✅ Deployed successfully!"