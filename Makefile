.PHONY: up down logs backend-test frontend-build fmt release-readiness release-clean-install-smoke release-upgrade-smoke release-worker-restart-smoke release-session-regression-smoke s3-sdk-smoke s3-edge-smoke s3-policy-smoke s3-policy-conditions-smoke support-bundle

up:
	docker compose --env-file .env up --build

down:
	docker compose down

logs:
	docker compose logs -f

backend-test:
	cd backend && go test ./...

frontend-build:
	cd frontend && npm run build

fmt:
	cd backend && gofmt -w ./cmd ./internal

release-readiness:
	powershell -ExecutionPolicy Bypass -File .\scripts\release-readiness.ps1

release-clean-install-smoke:
	powershell -ExecutionPolicy Bypass -File .\scripts\release-clean-install-smoke.ps1

release-upgrade-smoke:
	powershell -ExecutionPolicy Bypass -File .\scripts\release-upgrade-smoke.ps1

release-worker-restart-smoke:
	powershell -ExecutionPolicy Bypass -File .\scripts\release-worker-restart-smoke.ps1

release-session-regression-smoke:
	powershell -ExecutionPolicy Bypass -File .\scripts\release-session-regression-smoke.ps1

s3-sdk-smoke:
	powershell -ExecutionPolicy Bypass -File .\scripts\s3-sdk-smoke.ps1

s3-edge-smoke:
	powershell -ExecutionPolicy Bypass -File .\scripts\s3-edge-smoke.ps1

s3-policy-smoke:
	powershell -ExecutionPolicy Bypass -File .\scripts\s3-policy-smoke.ps1

s3-policy-conditions-smoke:
	powershell -ExecutionPolicy Bypass -File .\scripts\s3-policy-conditions-smoke.ps1

support-bundle:
	powershell -ExecutionPolicy Bypass -File .\scripts\support-bundle.ps1
