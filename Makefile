# Run all tests
.PHONY: test
test:
	go test ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	go test -cover ./...

# Run tests with detailed coverage report
.PHONY: test-coverage-html
test-coverage-html:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated at coverage.html"

# Clean build artifacts
.PHONY: clean
clean:
	rm -f cmdhooks
	rm -f coverage.out coverage.html
	rm -f *.log
	rm -f test*.sh

# Format code
.PHONY: fmt
fmt:
	go fmt ./...

# Run linter
.PHONY: lint
lint:
	golangci-lint run
